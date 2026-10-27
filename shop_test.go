package yespos_test

import (
	"context"
	"testing"

	yespos "github.com/Yes-Pos/yespos-client-sdk-go"
)

var (
	buyer   = yespos.Customer{Name: "Budi", Email: "budi@example.com", Phone: "0812"}
	testEDC = &yespos.EdcTerminal{AppCode: "YESPOS", TerminalCode: "T1", MerchantCode: "M1"}
)

func newTestShop(t *testing.T, mock *mockServer, options yespos.ShopOptions) *yespos.Shop {
	t.Helper()
	return yespos.NewShop(newTestClient(t, mock, nil), options)
}

// values memetakan metode pembayaran menjadi "<method>:<channel>".
func values(methods []yespos.PaymentMethod) []string {
	out := make([]string, 0, len(methods))
	for _, method := range methods {
		out = append(out, yespos.PaymentValue(method))
	}
	return out
}

func TestShopUsesConfiguredOutlet(t *testing.T) {
	mock := startMockServer(t)
	outlet, err := newTestShop(t, mock, yespos.ShopOptions{OutletUUID: "o2"}).Outlet(context.Background())
	if err != nil {
		t.Fatalf("Outlet: %v", err)
	}
	if outlet.UUID != "o2" {
		t.Fatalf("outlet = %q, mau o2", outlet.UUID)
	}
}

func TestShopFallsBackToFirstOutlet(t *testing.T) {
	mock := startMockServer(t)
	outlet, err := newTestShop(t, mock, yespos.ShopOptions{}).Outlet(context.Background())
	if err != nil {
		t.Fatalf("Outlet: %v", err)
	}
	if outlet.UUID != "o1" {
		t.Fatalf("outlet = %q, mau o1", outlet.UUID)
	}
}

func TestShopRefusesForeignOutlet(t *testing.T) {
	mock := startMockServer(t)
	_, err := newTestShop(t, mock, yespos.ShopOptions{OutletUUID: "nope"}).Outlet(context.Background())
	if status := statusOf(t, err); status != 404 {
		t.Fatalf("status = %d, mau 404", status)
	}
}

func TestShopHidesEDCWithoutTerminal(t *testing.T) {
	mock := startMockServer(t)
	methods, err := newTestShop(t, mock, yespos.ShopOptions{}).PaymentMethods(context.Background())
	if err != nil {
		t.Fatalf("PaymentMethods: %v", err)
	}
	got := values(methods)
	if len(got) != 2 || got[0] != "qris:qris" || got[1] != "va:bca" {
		t.Fatalf("metode = %v, mau [qris:qris va:bca]", got)
	}
}

func TestShopOffersEDCWithTerminal(t *testing.T) {
	mock := startMockServer(t)
	methods, err := newTestShop(t, mock, yespos.ShopOptions{EDC: testEDC}).PaymentMethods(context.Background())
	if err != nil {
		t.Fatalf("PaymentMethods: %v", err)
	}
	if !contains(values(methods), "edc:linkpayment") {
		t.Fatalf("metode = %v, mau memuat edc:linkpayment", values(methods))
	}
}

func TestShopPricesCartFromTheAPI(t *testing.T) {
	mock := startMockServer(t)
	items, err := newTestShop(t, mock, yespos.ShopOptions{}).PriceCart(context.Background(), []yespos.CartLine{
		{ProductUUID: "p1", Quantity: 2},
		{ProductUUID: "p2", VariantUUID: "v2", Quantity: 1},
	})
	if err != nil {
		t.Fatalf("PriceCart: %v", err)
	}
	want := []yespos.OrderItem{
		{ProductName: "Nasi Goreng", Quantity: 2, Price: 25000, Total: 50000},
		{ProductName: "Kopi Susu - Besar", Quantity: 1, Price: 24000, Total: 24000},
	}
	if len(items) != len(want) {
		t.Fatalf("item = %+v, mau %+v", items, want)
	}
	for i := range want {
		if items[i] != want[i] {
			t.Fatalf("item[%d] = %+v, mau %+v", i, items[i], want[i])
		}
	}
}

func TestShopRaisesQuantityToAtLeastOne(t *testing.T) {
	mock := startMockServer(t)
	items, err := newTestShop(t, mock, yespos.ShopOptions{}).PriceCart(context.Background(), []yespos.CartLine{
		{ProductUUID: "p1", Quantity: 2},
		{ProductUUID: "p1", Quantity: 0},
		{ProductUUID: "p1", Quantity: -3},
	})
	if err != nil {
		t.Fatalf("PriceCart: %v", err)
	}
	for i, want := range []int{2, 1, 1} {
		if items[i].Quantity != want {
			t.Fatalf("quantity[%d] = %d, mau %d", i, items[i].Quantity, want)
		}
	}
}

func TestShopRefusesUnsoldProductOrVariant(t *testing.T) {
	mock := startMockServer(t)
	shop := newTestShop(t, mock, yespos.ShopOptions{})
	for _, line := range []yespos.CartLine{
		{ProductUUID: "gone", Quantity: 1},
		{ProductUUID: "p2", VariantUUID: "gone", Quantity: 1},
	} {
		_, err := shop.PriceCart(context.Background(), []yespos.CartLine{line})
		if status := statusOf(t, err); status != 409 {
			t.Fatalf("%+v: status = %d, mau 409", line, status)
		}
	}
}

func TestShopPlacesOrderWithItsOwnTotal(t *testing.T) {
	mock := startMockServer(t)
	shop := newTestShop(t, mock, yespos.ShopOptions{NotifyURL: "https://example.com/cb"})

	order, err := shop.PlaceOrder(context.Background(), []yespos.CartLine{{ProductUUID: "p1", Quantity: 2}}, buyer, "va:bca")
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if order.TotalPayment != 50000 || order.PaymentMethod != "va" || order.PaymentNo != "8002300000123456" {
		t.Fatalf("order = %+v", order)
	}

	sent := mock.last(t).Body
	if sent["total_payment"] != float64(50000) || sent["payment_channel"] != "bca" || sent["name"] != "Budi" {
		t.Fatalf("body = %+v", sent)
	}
	if sent["notify_url"] != "https://example.com/cb" {
		t.Fatalf("notify_url = %v", sent["notify_url"])
	}
	// Terminal EDC tidak ikut terkirim untuk order non-EDC.
	if _, found := sent["app_code"]; found {
		t.Fatalf("app_code tidak boleh terkirim: %+v", sent)
	}
}

func TestShopSendsEDCTerminalOnlyForEDCOrders(t *testing.T) {
	mock := startMockServer(t)
	shop := newTestShop(t, mock, yespos.ShopOptions{EDC: testEDC})
	ctx := context.Background()
	line := []yespos.CartLine{{ProductUUID: "p1", Quantity: 1}}

	if _, err := shop.PlaceOrder(ctx, line, buyer, "edc:linkpayment"); err != nil {
		t.Fatalf("PlaceOrder edc: %v", err)
	}
	sent := mock.last(t).Body
	if sent["app_code"] != "YESPOS" || sent["terminal_code"] != "T1" || sent["merchant_code"] != "M1" {
		t.Fatalf("body edc = %+v", sent)
	}

	if _, err := shop.PlaceOrder(ctx, line, buyer, "qris:qris"); err != nil {
		t.Fatalf("PlaceOrder qris: %v", err)
	}
	if _, found := mock.last(t).Body["app_code"]; found {
		t.Fatalf("app_code tidak boleh terkirim untuk qris: %+v", mock.last(t).Body)
	}
}

func TestShopRefusesMethodThatIsNotActivated(t *testing.T) {
	mock := startMockServer(t)
	shop := newTestShop(t, mock, yespos.ShopOptions{})
	for _, payment := range []string{"va:mandiri", "edc:linkpayment", "cash:cash", "qris", ""} {
		_, err := shop.PlaceOrder(context.Background(), []yespos.CartLine{{ProductUUID: "p1", Quantity: 1}}, buyer, payment)
		if status := statusOf(t, err); status != 400 {
			t.Fatalf("%q: status = %d, mau 400", payment, status)
		}
	}
}

func TestShopChangesPaymentOfPendingOrder(t *testing.T) {
	mock := startMockServer(t)
	shop := newTestShop(t, mock, yespos.ShopOptions{})
	ctx := context.Background()

	order, err := shop.PlaceOrder(ctx, []yespos.CartLine{{ProductUUID: "p1", Quantity: 1}}, buyer, "qris:qris")
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	changed, err := shop.ChangePayment(ctx, order.UUID, "va:bca", nil)
	if err != nil {
		t.Fatalf("ChangePayment: %v", err)
	}
	if changed.PaymentMethod != "va" || changed.PaymentNo != "8002300000999999" || changed.QRString != "" {
		t.Fatalf("order = %+v", changed)
	}
	// Pembeli yang tersimpan bersama order dipakai ulang: tak ada yang perlu diisi lagi.
	if _, found := mock.last(t).Body["name"]; found {
		t.Fatalf("name tidak boleh terkirim: %+v", mock.last(t).Body)
	}
}

func TestShopSendsBuyerOnlyWhenGiven(t *testing.T) {
	mock := startMockServer(t)
	shop := newTestShop(t, mock, yespos.ShopOptions{})
	ctx := context.Background()

	order, err := shop.PlaceOrder(ctx, []yespos.CartLine{{ProductUUID: "p1", Quantity: 1}}, buyer, "qris:qris")
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	other := yespos.Customer{Name: "Citra", Email: "citra@example.com", Phone: "0814"}
	if _, err := shop.ChangePayment(ctx, order.UUID, "va:bca", &other); err != nil {
		t.Fatalf("ChangePayment: %v", err)
	}
	sent := mock.last(t).Body
	if sent["name"] != "Citra" || sent["phone"] != "0814" {
		t.Fatalf("body = %+v", sent)
	}
}

func TestShopCustomMessages(t *testing.T) {
	mock := startMockServer(t)
	shop := newTestShop(t, mock, yespos.ShopOptions{
		Messages: yespos.ShopMessages{MethodUnavailable: "Payment method unavailable."},
	})
	_, err := shop.PlaceOrder(context.Background(), []yespos.CartLine{{ProductUUID: "p1", Quantity: 1}}, buyer, "cash:cash")
	if err == nil || err.Error() != "Payment method unavailable." {
		t.Fatalf("pesan = %v", err)
	}
	// Pesan lain tetap memakai bawaannya.
	_, err = shop.PriceCart(context.Background(), []yespos.CartLine{{ProductUUID: "gone", Quantity: 1}})
	if err == nil || err.Error() == "Payment method unavailable." {
		t.Fatalf("pesan = %v", err)
	}
}

func TestShopFromEnv(t *testing.T) {
	mock := startMockServer(t)
	shop, err := yespos.ShopFromEnv(yespos.ShopOptions{}, yespos.MapEnv(map[string]string{
		"YESPOS_API_BASE_URL":      mock.URL,
		"YESPOS_CLIENT_KEY":        testKey,
		"YESPOS_CLIENT_SECRET":     testSecret,
		"YESPOS_OUTLET_UUID":       "o2",
		"YESPOS_EDC_APP_CODE":      testEDC.AppCode,
		"YESPOS_EDC_TERMINAL_CODE": testEDC.TerminalCode,
		"YESPOS_EDC_MERCHANT_CODE": testEDC.MerchantCode,
	}))
	if err != nil {
		t.Fatalf("ShopFromEnv: %v", err)
	}
	outlet, err := shop.Outlet(context.Background())
	if err != nil {
		t.Fatalf("Outlet: %v", err)
	}
	if outlet.UUID != "o2" {
		t.Fatalf("outlet = %q, mau o2", outlet.UUID)
	}
	methods, err := shop.PaymentMethods(context.Background())
	if err != nil {
		t.Fatalf("PaymentMethods: %v", err)
	}
	if !contains(values(methods), "edc:linkpayment") {
		t.Fatalf("metode = %v, mau memuat edc:linkpayment", values(methods))
	}
}

func TestShopFromEnvHidesHalfConfiguredEDC(t *testing.T) {
	mock := startMockServer(t)
	shop, err := yespos.ShopFromEnv(yespos.ShopOptions{}, yespos.MapEnv(map[string]string{
		"YESPOS_API_BASE_URL":  mock.URL,
		"YESPOS_CLIENT_KEY":    testKey,
		"YESPOS_CLIENT_SECRET": testSecret,
		"YESPOS_EDC_APP_CODE":  testEDC.AppCode,
	}))
	if err != nil {
		t.Fatalf("ShopFromEnv: %v", err)
	}
	methods, err := shop.PaymentMethods(context.Background())
	if err != nil {
		t.Fatalf("PaymentMethods: %v", err)
	}
	if contains(values(methods), "edc:linkpayment") {
		t.Fatalf("metode = %v, EDC harus disembunyikan", values(methods))
	}
}

func TestParseOrderState(t *testing.T) {
	cases := map[string]yespos.OrderState{
		"pending":           yespos.OrderStatePending,
		"pending_unsettled": yespos.OrderStatePending,
		"berhasil_settled":  yespos.OrderStatePaid,
		"success":           yespos.OrderStatePaid,
		"Berhasil":          yespos.OrderStatePaid,
		"expired":           yespos.OrderStateFailed,
	}
	for status, want := range cases {
		if got := yespos.ParseOrderState(status); got != want {
			t.Fatalf("ParseOrderState(%q) = %q, mau %q", status, got, want)
		}
	}
}

func TestProductIsSoldOut(t *testing.T) {
	base := yespos.Product{UUID: "x", Name: "x", Price: 1, Unit: "pcs", Type: "t"}

	notTracked := base
	if notTracked.IsSoldOut() {
		t.Fatal("produk tanpa pelacakan stok tidak pernah habis")
	}

	tracked := base
	tracked.TrackStock = true
	if !tracked.IsSoldOut() {
		t.Fatal("stok 0 harus dianggap habis")
	}

	inStock := tracked
	inStock.CurrentStock = 3
	if inStock.IsSoldOut() {
		t.Fatal("stok 3 tidak habis")
	}

	variants := []yespos.ProductVariant{
		{UUID: "a", Name: "a", Price: 1, CurrentStock: 0},
		{UUID: "b", Name: "b", Price: 2, CurrentStock: 4},
	}
	some := tracked
	some.Variants = variants
	if some.IsSoldOut() {
		t.Fatal("masih ada varian bersisa")
	}

	none := tracked
	none.Variants = variants[:1]
	if !none.IsSoldOut() {
		t.Fatal("semua varian habis")
	}
}

func TestProductPriceRange(t *testing.T) {
	base := yespos.Product{UUID: "x", Name: "x", Price: 10, Unit: "pcs", Type: "t"}
	if min, max := base.PriceRange(); min != 10 || max != 10 {
		t.Fatalf("PriceRange = %v..%v, mau 10..10", min, max)
	}

	withVariants := base
	withVariants.Variants = []yespos.ProductVariant{
		{UUID: "a", Name: "a", Price: 30, CurrentStock: 1},
		{UUID: "b", Name: "b", Price: 20, CurrentStock: 1},
	}
	if min, max := withVariants.PriceRange(); min != 20 || max != 30 {
		t.Fatalf("PriceRange = %v..%v, mau 20..30", min, max)
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
