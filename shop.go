package yespos

import (
	"context"
	"strings"
)

// ShopOptions mengatur perilaku [Shop].
type ShopOptions struct {
	// OutletUUID adalah outlet tempat berjualan. Dikosongkan, outlet pertama
	// milik pemilik client yang dipakai.
	OutletUUID string
	// NotifyURL adalah alamat callback status order dari YesPOS.
	NotifyURL string
	// EDC adalah terminal yang menagih pembayaran kartu; tanpa ini EDC
	// disembunyikan dari daftar metode pembayaran.
	EDC *EdcTerminal
	// Messages adalah pesan yang ditampilkan ke pembeli, agar storefront bisa
	// menerjemahkannya. Field kosong memakai pesan bawaan.
	Messages ShopMessages
}

// ShopMessages adalah pesan yang dilemparkan [Shop] sebagai error.
type ShopMessages struct {
	OutletNotFound     string
	ProductUnavailable string
	MethodUnavailable  string
}

var defaultMessages = ShopMessages{
	OutletNotFound:     "Outlet toko tidak ditemukan. Periksa OutletUUID.",
	ProductUnavailable: "Ada produk di keranjang yang sudah tidak tersedia. Hapus lalu coba lagi.",
	MethodUnavailable:  "Metode pembayaran tidak tersedia.",
}

func (m ShopMessages) withDefaults() ShopMessages {
	if m.OutletNotFound == "" {
		m.OutletNotFound = defaultMessages.OutletNotFound
	}
	if m.ProductUnavailable == "" {
		m.ProductUnavailable = defaultMessages.ProductUnavailable
	}
	if m.MethodUnavailable == "" {
		m.MethodUnavailable = defaultMessages.MethodUnavailable
	}
	return m
}

// Shop adalah storefront di atas [Client]: ia tahu outlet mana yang dipakai
// berjualan, menyembunyikan EDC bila tak ada terminal yang dikonfigurasi, dan —
// yang terpenting — menghitung harga setiap order dari API, sehingga keranjang
// yang diutak-atik tidak mengubah tagihan pembeli.
//
//	shop, err := yespos.NewShopWithOptions(clientOptions, yespos.ShopOptions{OutletUUID: outletUUID})
//	order, err := shop.PlaceOrder(ctx,
//	    []yespos.CartLine{{ProductUUID: "abc", Quantity: 2}},
//	    yespos.Customer{Name: "Budi", Email: "budi@example.com", Phone: "0812"},
//	    "va:bca",
//	)
type Shop struct {
	Client   *Client
	options  ShopOptions
	messages ShopMessages
}

// NewShop membungkus client yang sudah ada.
func NewShop(client *Client, options ShopOptions) *Shop {
	return &Shop{Client: client, options: options, messages: options.Messages.withDefaults()}
}

// NewShopWithOptions membuat client sendiri dari clientOptions.
func NewShopWithOptions(clientOptions ClientOptions, options ShopOptions) (*Shop, error) {
	client, err := NewClient(clientOptions)
	if err != nil {
		return nil, err
	}
	return NewShop(client, options), nil
}

// MediaURL mengembalikan URL publik sebuah gambar yang diunggah.
func (s *Shop) MediaURL(token string) string { return s.Client.MediaURL(token) }

// Outlet mengembalikan outlet tempat toko berjualan: OutletUUID, atau outlet
// pertama milik pemilik client.
func (s *Shop) Outlet(ctx context.Context) (*Outlet, error) {
	outlets, err := s.Client.Outlets.List(ctx)
	if err != nil {
		return nil, err
	}
	if s.options.OutletUUID == "" {
		if len(outlets) > 0 {
			return &outlets[0], nil
		}
		return nil, &Error{Status: 404, Message: s.messages.OutletNotFound}
	}
	for i := range outlets {
		if outlets[i].UUID == s.options.OutletUUID {
			return &outlets[i], nil
		}
	}
	return nil, &Error{Status: 404, Message: s.messages.OutletNotFound}
}

// Products mengembalikan produk outlet toko, dikelompokkan per kategori.
func (s *Shop) Products(ctx context.Context) ([]ProductCategory, error) {
	outlet, err := s.Outlet(ctx)
	if err != nil {
		return nil, err
	}
	return s.Client.Products.List(ctx, outlet.UUID)
}

// Product mengembalikan satu produk outlet toko.
func (s *Shop) Product(ctx context.Context, productUUID string) (*ProductDetail, error) {
	outlet, err := s.Outlet(ctx)
	if err != nil {
		return nil, err
	}
	return s.Client.Products.Get(ctx, outlet.UUID, productUUID)
}

// PaymentMethods mengembalikan metode yang diaktifkan; EDC hanya muncul bila
// terminalnya dikonfigurasi.
func (s *Shop) PaymentMethods(ctx context.Context) ([]PaymentMethod, error) {
	methods, err := s.Client.PaymentMethods.List(ctx)
	if err != nil {
		return nil, err
	}
	available := make([]PaymentMethod, 0, len(methods))
	for _, method := range methods {
		if method.PaymentMethod == PaymentMethodEDC && s.options.EDC == nil {
			continue
		}
		available = append(available, method)
	}
	return available, nil
}

// Order mengembalikan satu order milik toko.
func (s *Shop) Order(ctx context.Context, uuid string) (*Order, error) {
	return s.Client.Orders.Get(ctx, uuid)
}

// PriceCart mengubah baris keranjang menjadi item order, dengan harga diambil
// dari API. Gagal bila sebuah baris menunjuk produk atau varian yang sudah
// tidak dijual outlet.
func (s *Shop) PriceCart(ctx context.Context, lines []CartLine) ([]OrderItem, error) {
	outlet, err := s.Outlet(ctx)
	if err != nil {
		return nil, err
	}
	products, err := s.Client.Products.Map(ctx, outlet.UUID)
	if err != nil {
		return nil, err
	}

	items := make([]OrderItem, 0, len(lines))
	for _, line := range lines {
		product, found := products[line.ProductUUID]
		if !found {
			return nil, &Error{Status: 409, Message: s.messages.ProductUnavailable}
		}
		name, price := product.Name, product.Price
		if line.VariantUUID != "" {
			variant, ok := product.Variant(line.VariantUUID)
			if !ok {
				return nil, &Error{Status: 409, Message: s.messages.ProductUnavailable}
			}
			name, price = product.Name+" - "+variant.Name, variant.Price
		}
		quantity := line.Quantity
		if quantity < 1 {
			quantity = 1
		}
		items = append(items, OrderItem{
			ProductName: name,
			Quantity:    quantity,
			Price:       price,
			Total:       price * Amount(quantity),
		})
	}
	return items, nil
}

// PlaceOrder menempatkan order untuk baris keranjang yang diberikan. Harga
// datang dari API, bukan dari pemanggil, dan metode pembayarannya harus salah
// satu yang diaktifkan pemilik client.
//
// payment ditulis "<method>:<channel>", misalnya "va:bca".
func (s *Shop) PlaceOrder(ctx context.Context, lines []CartLine, customer Customer, payment string) (*Order, error) {
	items, err := s.PriceCart(ctx, lines)
	if err != nil {
		return nil, err
	}
	method, channel, err := s.resolvePayment(ctx, payment)
	if err != nil {
		return nil, err
	}

	total := Amount(0)
	for _, item := range items {
		total += item.Total
	}

	input := CreateOrderInput{
		Customer:       customer,
		TotalPayment:   total,
		PaymentMethod:  method,
		PaymentChannel: channel,
		Items:          items,
		NotifyURL:      s.options.NotifyURL,
	}
	if method == PaymentMethodEDC && s.options.EDC != nil {
		input.AppCode = s.options.EDC.AppCode
		input.TerminalCode = s.options.EDC.TerminalCode
		input.MerchantCode = s.options.EDC.MerchantCode
	}
	return s.Client.Orders.Create(ctx, input)
}

// ChangePayment memindahkan order pending ke metode aktif yang lain. YesPOS
// mempertahankan total, item, split, dan pembelinya, lalu mengembalikan
// instruksi pembayaran yang baru.
//
// customer boleh nil; isi hanya untuk order yang belum punya pembeli tersimpan.
func (s *Shop) ChangePayment(ctx context.Context, uuid, payment string, customer *Customer) (*Order, error) {
	method, channel, err := s.resolvePayment(ctx, payment)
	if err != nil {
		return nil, err
	}
	input := ChangeOrderPaymentInput{PaymentMethod: method, PaymentChannel: channel}
	if customer != nil && customer.Name != "" {
		input.Name, input.Email, input.Phone = customer.Name, customer.Email, customer.Phone
	}
	if method == PaymentMethodEDC && s.options.EDC != nil {
		input.AppCode = s.options.EDC.AppCode
		input.TerminalCode = s.options.EDC.TerminalCode
		input.MerchantCode = s.options.EDC.MerchantCode
	}
	return s.Client.Orders.ChangePaymentMethod(ctx, uuid, input)
}

// resolvePayment mencocokkan "<method>:<channel>" dengan metode yang aktif.
func (s *Shop) resolvePayment(ctx context.Context, payment string) (method, channel string, err error) {
	method, channel, _ = strings.Cut(payment, ":")
	available, err := s.PaymentMethods(ctx)
	if err != nil {
		return "", "", err
	}
	for _, item := range available {
		if item.PaymentMethod == method && item.PaymentChannel == channel {
			return method, channel, nil
		}
	}
	return "", "", &Error{Status: 400, Message: s.messages.MethodUnavailable}
}

// ShopFromEnv membuat toko dari variabel environment yang biasa dipakai:
// YESPOS_API_BASE_URL, YESPOS_CLIENT_KEY, YESPOS_CLIENT_SECRET,
// YESPOS_OUTLET_UUID, YESPOS_NOTIFY_URL, dan YESPOS_EDC_APP_CODE /
// _TERMINAL_CODE / _MERCHANT_CODE.
//
// Field options yang terisi menimpa nilai dari environment.
func ShopFromEnv(options ShopOptions, env ...EnvLookup) (*Shop, error) {
	get := lookupOrOS(env)
	if options.OutletUUID == "" {
		options.OutletUUID = get("YESPOS_OUTLET_UUID")
	}
	if options.NotifyURL == "" {
		options.NotifyURL = get("YESPOS_NOTIFY_URL")
	}
	if options.EDC == nil {
		appCode, terminalCode, merchantCode := get("YESPOS_EDC_APP_CODE"), get("YESPOS_EDC_TERMINAL_CODE"), get("YESPOS_EDC_MERCHANT_CODE")
		// Terminal yang hanya setengah terisi tetap menyembunyikan EDC.
		if appCode != "" && terminalCode != "" && merchantCode != "" {
			options.EDC = &EdcTerminal{AppCode: appCode, TerminalCode: terminalCode, MerchantCode: merchantCode}
		}
	}
	return NewShopWithOptions(ClientOptions{
		BaseURL: get("YESPOS_API_BASE_URL"),
		Key:     get("YESPOS_CLIENT_KEY"),
		Secret:  get("YESPOS_CLIENT_SECRET"),
	}, options)
}
