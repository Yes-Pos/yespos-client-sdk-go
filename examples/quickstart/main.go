// Command quickstart menampilkan isi toko dari YesPOS, lalu — bila diminta —
// membuat satu order dan mencetak instruksi pembayarannya.
//
// Jalankan:
//
//	export YESPOS_API_BASE_URL="https://api.yespos.id"
//	export YESPOS_CLIENT_KEY="…"
//	export YESPOS_CLIENT_SECRET="…"
//
//	go run ./examples/quickstart                                   # lihat katalog
//	go run ./examples/quickstart -order <uuid-produk> -payment va:bca
//	go run ./examples/quickstart -status <uuid-order>              # cek status
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	yespos "github.com/Yes-Pos/yespos-client-sdk-go"
)

func main() {
	var (
		outletUUID  = flag.String("outlet", "", "uuid outlet; kosong = outlet pertama")
		productUUID = flag.String("order", "", "uuid produk yang dipesan; kosong = hanya lihat katalog")
		variantUUID = flag.String("variant", "", "uuid varian produk, bila ada")
		quantity    = flag.Int("qty", 1, "jumlah yang dipesan")
		payment     = flag.String("payment", "qris:qris", `metode pembayaran, ditulis "<method>:<channel>"`)
		orderUUID   = flag.String("status", "", "uuid order yang ingin dicek statusnya")
		timeout     = flag.Duration("timeout", 30*time.Second, "batas waktu seluruh perintah")
	)
	flag.Parse()

	// Ctrl-C membatalkan request yang sedang berjalan, bukan membunuh proses
	// di tengah panggilan HTTP.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	// Kredensial dibaca dari environment; secret tidak pernah ikut ke mana-mana
	// selain header signature.
	shop, err := yespos.ShopFromEnv(yespos.ShopOptions{OutletUUID: *outletUUID})
	if err != nil {
		exit(err)
	}

	if *orderUUID != "" {
		order, err := shop.Order(ctx, *orderUUID)
		if err != nil {
			exit(err)
		}
		printOrder(order)
		return
	}

	if err := printCatalog(ctx, shop); err != nil {
		exit(err)
	}

	if *productUUID == "" {
		fmt.Println("\nTambahkan -order <uuid-produk> untuk membuat order percobaan.")
		return
	}

	// Harga tidak ikut dikirim: SDK mengambilnya dari API, lalu menghitung total
	// sendiri. Keranjang yang diutak-atik tidak mengubah tagihan.
	order, err := shop.PlaceOrder(ctx,
		[]yespos.CartLine{{ProductUUID: *productUUID, VariantUUID: *variantUUID, Quantity: *quantity}},
		yespos.Customer{Name: "Budi Contoh", Email: "budi@example.com", Phone: "08123456789"},
		*payment,
	)
	if err != nil {
		exit(err)
	}
	fmt.Println()
	printOrder(order)
	fmt.Printf("\nCek status: go run ./examples/quickstart -status %s\n", order.UUID)
}

func printCatalog(ctx context.Context, shop *yespos.Shop) error {
	outlet, err := shop.Outlet(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Outlet: %s (%s)\n\n", outlet.Name, outlet.UUID)

	categories, err := shop.Products(ctx)
	if err != nil {
		return err
	}
	for _, category := range categories {
		fmt.Printf("# %s\n", category.CategoryName)
		for _, product := range category.Products {
			product := product
			min, max := product.PriceRange()
			harga := rupiah(min)
			if max != min {
				harga = fmt.Sprintf("%s – %s", rupiah(min), rupiah(max))
			}
			status := ""
			if product.IsSoldOut() {
				status = "  [habis]"
			}
			fmt.Printf("  %-38s %-22s %s%s\n", product.UUID, harga, product.Name, status)
			for _, variant := range product.Variants {
				fmt.Printf("    varian %-29s %-22s %s\n", variant.UUID, rupiah(variant.Price), variant.Name)
			}
		}
		fmt.Println()
	}

	methods, err := shop.PaymentMethods(ctx)
	if err != nil {
		return err
	}
	fmt.Println("# Metode pembayaran aktif")
	for _, method := range methods {
		fmt.Printf("  %-18s %s\n", method.Value(), method.Name)
	}
	return nil
}

func printOrder(order *yespos.Order) {
	fmt.Printf("Order  : %s\n", order.UUID)
	fmt.Printf("Total  : %s\n", rupiah(order.TotalPayment))
	fmt.Printf("Status : %s (%s)\n", order.Status, order.State())

	// Tiap metode memberi petunjuk bayar di field yang berbeda.
	switch {
	case order.QRString != "":
		fmt.Printf("QRIS   : %s\n", order.QRString)
	case order.PaymentNo != "":
		fmt.Printf("VA     : %s a.n. %s\n", order.PaymentNo, order.PaymentName)
		if order.ExpiredAt != "" {
			fmt.Printf("Tempo  : %s\n", order.ExpiredAt)
		}
	case order.AppLink != "":
		fmt.Printf("EDC    : %s\n", order.AppLink)
	}
}

// rupiah memformat nominal menjadi "Rp25.000".
func rupiah(amount yespos.Amount) string {
	digits := fmt.Sprintf("%d", amount)
	negative := strings.HasPrefix(digits, "-")
	digits = strings.TrimPrefix(digits, "-")

	var grouped strings.Builder
	for i, digit := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			grouped.WriteByte('.')
		}
		grouped.WriteRune(digit)
	}
	if negative {
		return "-Rp" + grouped.String()
	}
	return "Rp" + grouped.String()
}

// exit menampilkan kegagalan SDK dengan status HTTP-nya, lalu berhenti.
func exit(err error) {
	var sdkError *yespos.Error
	if errors.As(err, &sdkError) {
		fmt.Fprintf(os.Stderr, "yespos: %s (status %d)\n", sdkError.Message, sdkError.Status)
		if sdkError.IsAuthError() {
			fmt.Fprintln(os.Stderr, "Periksa YESPOS_CLIENT_KEY dan YESPOS_CLIENT_SECRET.")
		}
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
