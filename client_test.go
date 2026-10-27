package yespos_test

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"testing"
	"time"

	yespos "github.com/Yes-Pos/yespos-client-sdk-go"
)

func newTestClient(t *testing.T, mock *mockServer, tweak func(*yespos.ClientOptions)) *yespos.Client {
	t.Helper()
	options := yespos.ClientOptions{BaseURL: mock.URL, Key: testKey, Secret: testSecret}
	if tweak != nil {
		tweak(&options)
	}
	client, err := yespos.NewClient(options)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

// statusOf mengambil status HTTP dari sebuah error SDK.
func statusOf(t *testing.T, err error) int {
	t.Helper()
	sdkError, ok := yespos.AsError(err)
	if !ok {
		t.Fatalf("error bukan *yespos.Error: %v", err)
	}
	return sdkError.Status
}

func TestSignMatchesBackend(t *testing.T) {
	// Vektor tetap: backend menghashkan body kosong sebagai "{}".
	if got, want := yespos.Sign("GET", "k", "s", ""), yespos.Sign("GET", "k", "s", "{}"); got != want {
		t.Fatalf("body kosong harus ditandatangani seperti {}: %s != %s", got, want)
	}
	signature := yespos.Sign("POST", "k", "s", `{"a":1}`)
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(signature) {
		t.Fatalf("signature bukan hex 64 karakter: %q", signature)
	}
	// Method dan body sama-sama mengubah tanda tangan.
	if yespos.Sign("GET", "k", "s", `{"a":1}`) == signature {
		t.Fatal("method harus mengubah signature")
	}
	if yespos.Sign("POST", "k", "s", `{"a":2}`) == signature {
		t.Fatal("body harus mengubah signature")
	}
	// Huruf kecil pada method diperlakukan sama dengan huruf besar.
	if yespos.Sign("post", "k", "s", `{"a":1}`) != signature {
		t.Fatal("method harus di-uppercase sebelum ditandatangani")
	}
}

func TestSignatureAcceptedByServer(t *testing.T) {
	mock := startMockServer(t)
	client := newTestClient(t, mock, nil)

	outlets, err := client.Outlets.List(context.Background())
	if err != nil {
		t.Fatalf("Outlets.List: %v", err)
	}
	if len(outlets) != 2 {
		t.Fatalf("outlet = %d, mau 2", len(outlets))
	}
}

func TestSignatureRejectedWhenSecretWrong(t *testing.T) {
	mock := startMockServer(t)
	client := newTestClient(t, mock, func(o *yespos.ClientOptions) { o.Secret = "nope" })

	_, err := client.Outlets.List(context.Background())
	if status := statusOf(t, err); status != 401 {
		t.Fatalf("status = %d, mau 401", status)
	}
}

func TestClientRefusesToBuildWithoutCredentials(t *testing.T) {
	if _, err := yespos.NewClient(yespos.ClientOptions{}); err == nil {
		t.Fatal("client tanpa kredensial harus gagal dibuat")
	} else if !yespos.IsError(err) {
		t.Fatalf("error bukan *yespos.Error: %v", err)
	}
}

func TestClientTrimsTrailingSlashAndBuildsMediaURL(t *testing.T) {
	mock := startMockServer(t)
	client := newTestClient(t, mock, func(o *yespos.ClientOptions) { o.BaseURL = mock.URL + "///" })

	if got, want := client.MediaURL("2_abc"), mock.URL+"/media/2_abc"; got != want {
		t.Fatalf("MediaURL = %q, mau %q", got, want)
	}
}

func TestClientSendsConfiguredLanguage(t *testing.T) {
	mock := startMockServer(t)
	client := newTestClient(t, mock, func(o *yespos.ClientOptions) { o.Language = "en" })

	if _, err := client.Outlets.List(context.Background()); err != nil {
		t.Fatalf("Outlets.List: %v", err)
	}
	if got := mock.last(t).Language; got != "en" {
		t.Fatalf("Accept-Language = %q, mau \"en\"", got)
	}

	// Tanpa pengaturan, bahasa bawaannya "id".
	if _, err := newTestClient(t, mock, nil).Outlets.List(context.Background()); err != nil {
		t.Fatalf("Outlets.List: %v", err)
	}
	if got := mock.last(t).Language; got != yespos.DefaultLanguage {
		t.Fatalf("Accept-Language = %q, mau %q", got, yespos.DefaultLanguage)
	}
}

func TestProductsListAndMap(t *testing.T) {
	mock := startMockServer(t)
	client := newTestClient(t, mock, nil)
	ctx := context.Background()

	list, err := client.Products.List(ctx, "o1")
	if err != nil {
		t.Fatalf("Products.List: %v", err)
	}
	names := []string{}
	for _, category := range list {
		names = append(names, category.CategoryName)
	}
	if len(names) != 2 || names[0] != "Makanan" || names[1] != "Favorit" {
		t.Fatalf("kategori = %v, mau [Makanan Favorit]", names)
	}

	products, err := client.Products.Map(ctx, "o1")
	if err != nil {
		t.Fatalf("Products.Map: %v", err)
	}
	// Nasi Goreng ada di dua kategori tapi hanya tercatat sekali.
	if len(products) != 2 {
		t.Fatalf("produk = %d, mau 2", len(products))
	}
	if _, ok := products["p1"]; !ok {
		t.Fatal("p1 tidak ada di map")
	}
	if products["p1"].Price != 25000 {
		t.Fatalf("harga p1 = %v, mau 25000", products["p1"].Price)
	}
}

func TestProductGetIncludesCategories(t *testing.T) {
	mock := startMockServer(t)
	client := newTestClient(t, mock, nil)

	product, err := client.Products.Get(context.Background(), "o1", "p2")
	if err != nil {
		t.Fatalf("Products.Get: %v", err)
	}
	if product.Name != "Kopi Susu" {
		t.Fatalf("nama = %q, mau \"Kopi Susu\"", product.Name)
	}
	if len(product.Categories) != 1 || product.Categories[0] != "Makanan" {
		t.Fatalf("kategori = %v, mau [Makanan]", product.Categories)
	}
	if len(product.Variants) != 2 {
		t.Fatalf("varian = %d, mau 2", len(product.Variants))
	}
}

func TestProductGetUnknownIs404(t *testing.T) {
	mock := startMockServer(t)
	client := newTestClient(t, mock, nil)

	_, err := client.Products.Get(context.Background(), "o1", "nope")
	sdkError, ok := yespos.AsError(err)
	if !ok {
		t.Fatalf("error bukan *yespos.Error: %v", err)
	}
	if sdkError.Status != 404 {
		t.Fatalf("status = %d, mau 404", sdkError.Status)
	}
	if sdkError.Message != "Produk tidak ditemukan." {
		t.Fatalf("pesan = %q", sdkError.Message)
	}
}

func TestOrdersCreateGetList(t *testing.T) {
	mock := startMockServer(t)
	client := newTestClient(t, mock, nil)
	ctx := context.Background()

	order, err := client.Orders.Create(ctx, yespos.CreateOrderInput{
		Customer:       yespos.Customer{Name: "Budi", Email: "budi@example.com", Phone: "0812"},
		TotalPayment:   50000,
		PaymentMethod:  "qris",
		PaymentChannel: "qris",
		Items:          []yespos.OrderItem{{ProductName: "Nasi Goreng", Quantity: 2, Price: 25000, Total: 50000}},
	})
	if err != nil {
		t.Fatalf("Orders.Create: %v", err)
	}
	if order.QRString == "" {
		t.Fatal("order QRIS harus membawa qr_string")
	}

	fetched, err := client.Orders.Get(ctx, order.UUID)
	if err != nil {
		t.Fatalf("Orders.Get: %v", err)
	}
	if !fetched.IsPending() {
		t.Fatalf("status = %q, mau pending", fetched.Status)
	}

	list, err := client.Orders.List(ctx, yespos.OrderListParams{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("Orders.List: %v", err)
	}
	if list.Page != 1 || list.Total != 1 {
		t.Fatalf("list = %+v, mau page 1 total 1", list)
	}
}

func TestOrdersChangePaymentMethod(t *testing.T) {
	mock := startMockServer(t)
	client := newTestClient(t, mock, nil)
	ctx := context.Background()

	order, err := client.Orders.Create(ctx, yespos.CreateOrderInput{
		Customer:       yespos.Customer{Name: "Budi", Email: "budi@example.com", Phone: "0812"},
		TotalPayment:   25000,
		PaymentMethod:  "qris",
		PaymentChannel: "qris",
		Items:          []yespos.OrderItem{{ProductName: "Nasi Goreng", Quantity: 1, Price: 25000, Total: 25000}},
	})
	if err != nil {
		t.Fatalf("Orders.Create: %v", err)
	}

	changed, err := client.Orders.ChangePaymentMethod(ctx, order.UUID, yespos.ChangeOrderPaymentInput{
		PaymentMethod: "va", PaymentChannel: "bca",
	})
	if err != nil {
		t.Fatalf("ChangePaymentMethod: %v", err)
	}
	if changed.PaymentMethod != "va" || changed.PaymentNo != "8002300000999999" {
		t.Fatalf("order = %+v", changed)
	}
	if changed.QRString != "" {
		t.Fatalf("qr_string harus hilang, dapat %q", changed.QRString)
	}
}

func TestErrorJoinsValidationMessages(t *testing.T) {
	mock := startMockServer(t)
	client := newTestClient(t, mock, nil)

	mock.failNext(400, map[string]any{"message": []string{"payment_method:required", "email:email"}})
	_, err := client.Outlets.List(context.Background())
	if err == nil || err.Error() != "payment_method:required email:email" {
		t.Fatalf("pesan = %v", err)
	}
}

func TestErrorKeepsDetails(t *testing.T) {
	mock := startMockServer(t)
	client := newTestClient(t, mock, nil)

	mock.failNext(422, map[string]any{
		"message": "tidak lengkap",
		"details": []map[string]any{{"uuid": "p1", "missing": []string{"image"}}},
	})
	_, err := client.Outlets.List(context.Background())
	sdkError, ok := yespos.AsError(err)
	if !ok {
		t.Fatalf("error bukan *yespos.Error: %v", err)
	}
	var details []struct {
		UUID    string   `json:"uuid"`
		Missing []string `json:"missing"`
	}
	if err := sdkError.DecodeDetails(&details); err != nil {
		t.Fatalf("DecodeDetails: %v", err)
	}
	if len(details) != 1 || details[0].UUID != "p1" || details[0].Missing[0] != "image" {
		t.Fatalf("details = %+v", details)
	}
}

func TestErrorFallsBackToAMessage(t *testing.T) {
	mock := startMockServer(t)
	client := newTestClient(t, mock, nil)

	mock.failNext(500, map[string]any{})
	_, err := client.Outlets.List(context.Background())
	if err == nil || err.Error() != "Permintaan gagal (500)." {
		t.Fatalf("pesan = %v", err)
	}
}

func TestUnreachableServerIsNetworkError(t *testing.T) {
	client, err := yespos.NewClient(yespos.ClientOptions{BaseURL: "http://127.0.0.1:1", Key: testKey, Secret: testSecret})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.Outlets.List(context.Background())
	sdkError, ok := yespos.AsError(err)
	if !ok {
		t.Fatalf("error bukan *yespos.Error: %v", err)
	}
	if !sdkError.IsNetworkError() {
		t.Fatalf("IsNetworkError = false untuk %+v", sdkError)
	}
}

func TestSlowRequestTimesOut(t *testing.T) {
	mock := startMockServer(t)
	mock.sleepFor(200 * time.Millisecond)
	client := newTestClient(t, mock, func(o *yespos.ClientOptions) { o.Timeout = 20 * time.Millisecond })

	_, err := client.Outlets.List(context.Background())
	if err == nil || err.Error() != "Permintaan ke YesPOS melebihi batas waktu." {
		t.Fatalf("pesan = %v", err)
	}
	if sdkError, _ := yespos.AsError(err); sdkError == nil || !sdkError.IsNetworkError() {
		t.Fatalf("timeout harus dihitung sebagai network error: %v", err)
	}
}

func TestCanceledContextStopsTheRequest(t *testing.T) {
	mock := startMockServer(t)
	client := newTestClient(t, mock, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Outlets.List(ctx); err == nil {
		t.Fatal("context yang dibatalkan harus menggagalkan request")
	}
}

func TestClientFromEnv(t *testing.T) {
	mock := startMockServer(t)
	client, err := yespos.ClientFromEnv(yespos.MapEnv(map[string]string{
		"YESPOS_API_BASE_URL":  mock.URL,
		"YESPOS_CLIENT_KEY":    testKey,
		"YESPOS_CLIENT_SECRET": testSecret,
	}))
	if err != nil {
		t.Fatalf("ClientFromEnv: %v", err)
	}
	if _, err := client.Outlets.List(context.Background()); err != nil {
		t.Fatalf("Outlets.List: %v", err)
	}
}

func TestHTTPEscapeHatch(t *testing.T) {
	mock := startMockServer(t)
	client := newTestClient(t, mock, nil)

	// Endpoint yang belum dibungkus SDK tetap bisa dipanggil lewat transportnya.
	var outlets []yespos.Outlet
	if err := client.HTTP.Get(context.Background(), "/outlets", nil, &outlets); err != nil {
		t.Fatalf("HTTP.Get: %v", err)
	}
	if len(outlets) != 2 {
		t.Fatalf("outlet = %d, mau 2", len(outlets))
	}
}

func TestAmountAcceptsNumbersAndStrings(t *testing.T) {
	for _, raw := range []string{`25000`, `25000.4`, `"25000"`, `"25000.00"`} {
		var amount yespos.Amount
		if err := json.Unmarshal([]byte(raw), &amount); err != nil {
			t.Fatalf("Unmarshal(%s): %v", raw, err)
		}
		if amount != 25000 {
			t.Fatalf("Unmarshal(%s) = %v, mau 25000", raw, amount)
		}
	}
	var zero yespos.Amount
	if err := json.Unmarshal([]byte(`null`), &zero); err != nil || zero != 0 {
		t.Fatalf("null harus jadi 0: %v %v", zero, err)
	}
	encoded, err := json.Marshal(yespos.Amount(25000))
	if err != nil || string(encoded) != "25000" {
		t.Fatalf("Marshal = %s, %v", encoded, err)
	}
}

// Transport kustom tetap dipakai, misalnya untuk proxy atau retry.
func TestCustomHTTPClientIsUsed(t *testing.T) {
	mock := startMockServer(t)
	counter := &countingTransport{base: http.DefaultTransport}
	client := newTestClient(t, mock, func(o *yespos.ClientOptions) {
		o.HTTPClient = &http.Client{Transport: counter}
	})

	if _, err := client.Outlets.List(context.Background()); err != nil {
		t.Fatalf("Outlets.List: %v", err)
	}
	if counter.calls != 1 {
		t.Fatalf("transport dipanggil %d kali, mau 1", counter.calls)
	}
}

type countingTransport struct {
	base  http.RoundTripper
	calls int
}

func (t *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.calls++
	return t.base.RoundTrip(r)
}
