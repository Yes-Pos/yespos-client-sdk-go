// Command storefront adalah toko online kecil di atas SDK YesPOS.
//
// Polanya sama dengan contoh landing page lain: SDK hanya hidup di server, dan
// browser memanggil endpoint JSON milik aplikasi sendiri. Secret tidak pernah
// meninggalkan proses ini, dan harga selalu dihitung ulang dari API — keranjang
// yang diutak-atik di browser tidak mengubah tagihan pembeli.
//
// Jalankan:
//
//	export YESPOS_API_BASE_URL="https://api.yespos.id"
//	export YESPOS_CLIENT_KEY="…"
//	export YESPOS_CLIENT_SECRET="…"
//	export YESPOS_OUTLET_UUID="…"          # opsional
//	export YESPOS_NOTIFY_URL="https://…/api/notify"   # opsional
//
//	go run ./examples/storefront        # lalu buka http://localhost:8080
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	yespos "github.com/Yes-Pos/yespos-client-sdk-go"
)

//go:embed index.html
var indexPage []byte

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	// Satu Shop dipakai bersama oleh semua request: aman dipanggil paralel.
	shop, err := yespos.ShopFromEnv(yespos.ShopOptions{})
	if err != nil {
		log.Fatalf("konfigurasi YesPOS belum lengkap: %v", err)
	}
	store := &storefront{shop: shop}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexPage)
	})
	mux.HandleFunc("GET /api/catalog", store.catalog)
	mux.HandleFunc("GET /api/payment-methods", store.paymentMethods)
	mux.HandleFunc("POST /api/checkout", store.checkout)
	mux.HandleFunc("GET /api/orders/{uuid}", store.order)
	mux.HandleFunc("POST /api/notify", store.notify)

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("storefront jalan di http://localhost%s", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
	log.Println("storefront berhenti")
}

type storefront struct{ shop *yespos.Shop }

// catalog mengirim outlet dan produknya ke browser — tanpa data internal.
func (s *storefront) catalog(w http.ResponseWriter, r *http.Request) {
	outlet, err := s.shop.Outlet(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	categories, err := s.shop.Products(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}

	type variantView struct {
		UUID    string        `json:"uuid"`
		Name    string        `json:"name"`
		Price   yespos.Amount `json:"price"`
		SoldOut bool          `json:"sold_out"`
	}
	type productView struct {
		UUID     string        `json:"uuid"`
		Name     string        `json:"name"`
		Price    yespos.Amount `json:"price"`
		PriceMax yespos.Amount `json:"price_max"`
		Image    string        `json:"image,omitempty"`
		SoldOut  bool          `json:"sold_out"`
		Variants []variantView `json:"variants,omitempty"`
	}
	type categoryView struct {
		Name     string        `json:"name"`
		Products []productView `json:"products"`
	}

	view := make([]categoryView, 0, len(categories))
	for _, category := range categories {
		products := make([]productView, 0, len(category.Products))
		for _, product := range category.Products {
			product := product
			min, max := product.PriceRange()

			image := product.MainImageURL
			// Produk lama hanya menyimpan token; URL publiknya dibangun di sini.
			if image == "" && product.MainImageToken != "" {
				image = s.shop.MediaURL(product.MainImageToken)
			}

			variants := make([]variantView, 0, len(product.Variants))
			for _, variant := range product.Variants {
				variants = append(variants, variantView{
					UUID:    variant.UUID,
					Name:    variant.Name,
					Price:   variant.Price,
					SoldOut: product.TrackStock && variant.CurrentStock <= 0,
				})
			}

			products = append(products, productView{
				UUID:     product.UUID,
				Name:     product.Name,
				Price:    min,
				PriceMax: max,
				Image:    image,
				SoldOut:  product.IsSoldOut(),
				Variants: variants,
			})
		}
		view = append(view, categoryView{Name: category.CategoryName, Products: products})
	}

	writeJSON(w, http.StatusOK, map[string]any{"outlet": outlet.Name, "categories": view})
}

// paymentMethods hanya mengirim metode yang benar-benar bisa dipakai: EDC
// disembunyikan Shop bila terminalnya tidak dikonfigurasi.
func (s *storefront) paymentMethods(w http.ResponseWriter, r *http.Request) {
	methods, err := s.shop.PaymentMethods(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	type methodView struct {
		Value string `json:"value"`
		Label string `json:"label"`
	}
	view := make([]methodView, 0, len(methods))
	for _, method := range methods {
		view = append(view, methodView{Value: method.Value(), Label: method.Name})
	}
	writeJSON(w, http.StatusOK, view)
}

// checkoutRequest adalah apa yang dikirim browser: tidak ada harga di dalamnya.
type checkoutRequest struct {
	Lines []struct {
		ProductUUID string `json:"product_uuid"`
		VariantUUID string `json:"variant_uuid"`
		Quantity    int    `json:"quantity"`
	} `json:"lines"`
	Customer yespos.Customer `json:"customer"`
	Payment  string          `json:"payment"`
}

func (s *storefront) checkout(w http.ResponseWriter, r *http.Request) {
	var request checkoutRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Data checkout tidak terbaca."})
		return
	}
	if len(request.Lines) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Keranjang masih kosong."})
		return
	}
	if request.Customer.Name == "" || request.Customer.Email == "" || request.Customer.Phone == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Nama, email, dan nomor telepon wajib diisi."})
		return
	}

	lines := make([]yespos.CartLine, 0, len(request.Lines))
	for _, line := range request.Lines {
		lines = append(lines, yespos.CartLine{
			ProductUUID: line.ProductUUID,
			VariantUUID: line.VariantUUID,
			Quantity:    line.Quantity,
		})
	}

	// Harga dan total dihitung SDK dari API, bukan dari body di atas.
	order, err := s.shop.PlaceOrder(r.Context(), lines, request.Customer, request.Payment)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, orderView(order))
}

func (s *storefront) order(w http.ResponseWriter, r *http.Request) {
	order, err := s.shop.Order(r.Context(), r.PathValue("uuid"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orderView(order))
}

// notify menerima callback status order dari YesPOS (YESPOS_NOTIFY_URL).
//
// Aplikasi sungguhan memakai ini untuk memperbarui pesanannya sendiri, lalu
// membaca ulang order lewat SDK sebagai sumber kebenaran — jangan percaya
// begitu saja isi body callback.
func (s *storefront) notify(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		UUID   string `json:"uuid"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Printf("callback: order %s berstatus %q", payload.UUID, payload.Status)

	if payload.UUID != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if order, err := s.shop.Order(ctx, payload.UUID); err == nil {
			log.Printf("callback: status terverifikasi %q (%s)", order.Status, order.State())
		}
	}
	w.WriteHeader(http.StatusOK)
}

// orderView adalah bagian order yang aman ditampilkan ke pembeli.
func orderView(order *yespos.Order) map[string]any {
	view := map[string]any{
		"uuid":           order.UUID,
		"total_payment":  order.TotalPayment,
		"payment_method": order.PaymentMethod,
		"status":         order.Status,
		"state":          order.State(),
	}
	// Tiap metode memberi petunjuk bayar di field yang berbeda.
	switch {
	case order.QRString != "":
		view["qr_string"] = order.QRString
	case order.PaymentNo != "":
		view["payment_no"] = order.PaymentNo
		view["payment_name"] = order.PaymentName
		view["expired_at"] = order.ExpiredAt
	case order.AppLink != "":
		view["app_link"] = order.AppLink
	}
	return view
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// writeError meneruskan status dan pesan dari YesPOS apa adanya, kecuali
// kegagalan kredensial — itu masalah server, bukan urusan pembeli.
func writeError(w http.ResponseWriter, err error) {
	var sdkError *yespos.Error
	if !errors.As(err, &sdkError) {
		log.Printf("error tak terduga: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "Terjadi kesalahan."})
		return
	}
	switch {
	case sdkError.IsAuthError():
		log.Printf("kredensial YesPOS ditolak: %v", sdkError)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "Toko sedang tidak dapat melayani pesanan."})
	case sdkError.IsNetworkError():
		log.Printf("YesPOS tidak terjangkau: %v", sdkError)
		writeJSON(w, http.StatusBadGateway, map[string]string{"message": "Server pembayaran sedang tidak dapat dihubungi."})
	default:
		writeJSON(w, sdkError.Status, map[string]string{"message": sdkError.Message})
	}
}
