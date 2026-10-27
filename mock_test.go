package yespos_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testKey    = "test-key"
	testSecret = "test-secret"
)

// recorded adalah apa yang dicatat mock, agar test bisa memeriksa apa yang
// dikirim SDK.
type recorded struct {
	Method   string
	Path     string
	Body     map[string]any
	Language string
}

type failure struct {
	status int
	body   any
}

// mockServer adalah YesPOS client API secukupnya untuk menguji SDK: ia
// memverifikasi tanda tangan persis seperti ClientAuthMiddleware.
type mockServer struct {
	URL string

	mu       sync.Mutex
	server   *httptest.Server
	requests []recorded
	orders   map[string]map[string]any
	next     *failure
	delay    time.Duration
}

var categories = []map[string]any{
	{
		"category_uuid": "c1",
		"category_name": "Makanan",
		"products": []map[string]any{
			{
				"uuid": "p1", "name": "Nasi Goreng", "price": 25000, "unit": "porsi",
				"type": "fnb_main_product", "current_stock": 0, "track_stock": false,
				"main_image_token": "img1", "main_image_url": "https://cdn.example.com/media/img1",
				"variants": []map[string]any{},
			},
			{
				"uuid": "p2", "name": "Kopi Susu", "price": 18000, "unit": "cup",
				"type": "fnb_main_product", "current_stock": 0, "track_stock": true,
				"variants": []map[string]any{
					{"uuid": "v1", "name": "Kecil", "price": 18000, "current_stock": 5},
					{"uuid": "v2", "name": "Besar", "price": 24000, "current_stock": 0},
				},
			},
		},
	},
	// Satu produk bisa berada di beberapa kategori; SDK harus mencatatnya sekali.
	{"category_uuid": "c2", "category_name": "Favorit", "products": []map[string]any{}},
}

var paymentMethods = []map[string]any{
	{"name": "QRIS", "type": "qris", "issuer": "iPaymu", "payment_method": "qris", "payment_channel": "qris", "required_fields": []string{}},
	{"name": "BCA", "type": "bank_transfer", "issuer": "iPaymu", "payment_method": "va", "payment_channel": "bca", "required_fields": []string{}},
	{
		"name": "Credit Card", "type": "credit_card", "issuer": "TSM",
		"payment_method": "edc", "payment_channel": "linkpayment",
		"required_fields": []string{"app_code", "terminal_code", "merchant_code"},
	},
}

func init() {
	first := categories[0]["products"].([]map[string]any)[0]
	categories[1]["products"] = []map[string]any{first}
}

var (
	productPath  = regexp.MustCompile(`^/outlets/[^/]+/products$`)
	oneProduct   = regexp.MustCompile(`^/outlets/[^/]+/products/(.+)$`)
	changeMethod = regexp.MustCompile(`^/orders/([^/]+)/payment-method$`)
	oneOrder     = regexp.MustCompile(`^/orders/(.+)$`)
)

func startMockServer(t *testing.T) *mockServer {
	t.Helper()
	mock := &mockServer{orders: map[string]map[string]any{}}
	mock.server = httptest.NewServer(http.HandlerFunc(mock.handle))
	mock.URL = mock.server.URL
	t.Cleanup(mock.server.Close)
	return mock
}

// failNext menjawab request berikutnya dengan status dan body ini, lalu kembali normal.
func (m *mockServer) failNext(status int, body any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next = &failure{status: status, body: body}
}

// sleepFor membuat setiap request tertahan selama d, untuk menguji timeout.
func (m *mockServer) sleepFor(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delay = d
}

// last mengembalikan request terakhir yang dicatat.
func (m *mockServer) last(t *testing.T) recorded {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.requests) == 0 {
		t.Fatal("mock belum mencatat request apa pun")
	}
	return m.requests[len(m.requests)-1]
}

func (m *mockServer) handle(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	delay := m.delay
	m.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}

	body := readAll(r)

	writeJSON := func(status int, payload any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(payload)
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/client")

	if strings.HasPrefix(r.URL.Path, "/media/") {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png"))
		return
	}

	// Cek yang sama dengan backend: HMAC atas METHOD:key:sha256(body):secret.
	signed := strings.TrimSpace(body)
	if signed == "" {
		signed = "{}"
	}
	bodyHash := sha256.Sum256([]byte(signed))
	mac := hmac.New(sha256.New, []byte(testSecret))
	fmt.Fprintf(mac, "%s:%s:%s:%s", r.Method, testKey, hex.EncodeToString(bodyHash[:]), testSecret)
	if r.Header.Get("va") != testKey || r.Header.Get("signature") != hex.EncodeToString(mac.Sum(nil)) {
		writeJSON(401, map[string]any{"message": "Signature Tidak Sesuai (Invalid signature)"})
		return
	}

	entry := recorded{Method: r.Method, Path: path, Language: r.Header.Get("Accept-Language")}
	if body != "" {
		_ = json.Unmarshal([]byte(body), &entry.Body)
	}
	m.mu.Lock()
	m.requests = append(m.requests, entry)
	next := m.next
	m.next = nil
	m.mu.Unlock()

	if next != nil {
		writeJSON(next.status, next.body)
		return
	}

	switch {
	case r.Method == http.MethodGet && path == "/outlets":
		writeJSON(200, map[string]any{"data": []map[string]any{
			{"uuid": "o1", "name": "Cabang Sudirman"},
			{"uuid": "o2", "name": "Cabang Thamrin"},
		}})

	case r.Method == http.MethodGet && productPath.MatchString(path):
		writeJSON(200, map[string]any{"data": categories})

	case r.Method == http.MethodGet && oneProduct.MatchString(path):
		wanted := oneProduct.FindStringSubmatch(path)[1]
		for _, category := range categories {
			for _, product := range category["products"].([]map[string]any) {
				if product["uuid"] == wanted {
					detail := map[string]any{"categories": []string{"Makanan"}}
					for key, value := range product {
						detail[key] = value
					}
					writeJSON(200, map[string]any{"data": detail})
					return
				}
			}
		}
		writeJSON(404, map[string]any{"message": "Produk tidak ditemukan."})

	case r.Method == http.MethodGet && path == "/payment-method":
		writeJSON(200, map[string]any{"data": paymentMethods})

	case r.Method == http.MethodGet && path == "/orders":
		m.mu.Lock()
		list := make([]map[string]any, 0, len(m.orders))
		for _, order := range m.orders {
			list = append(list, order)
		}
		total := len(m.orders)
		m.mu.Unlock()
		writeJSON(200, map[string]any{"data": map[string]any{"data": list, "total": total, "page": 1, "limit": 10}})

	case r.Method == http.MethodPost && path == "/orders":
		var input map[string]any
		_ = json.Unmarshal([]byte(body), &input)
		uuid := fmt.Sprintf("ord-%d", time.Now().UnixNano())
		order := map[string]any{
			"uuid":            uuid,
			"total_payment":   input["total_payment"],
			"payment_method":  input["payment_method"],
			"payment_channel": input["payment_channel"],
			"status":          "pending",
			"items":           input["items"],
			"customer":        map[string]any{"name": input["name"], "email": input["email"], "phone": input["phone"]},
			"created_at":      time.Now().Format(time.RFC3339),
		}
		switch input["payment_method"] {
		case "qris":
			order["qr_string"] = "00020101021226"
		case "va":
			order["payment_no"] = "8002300000123456"
			order["payment_name"] = "IPAYMU"
		case "edc":
			order["app_link"] = "https://erpay.id/payment?data=ABC"
		}
		m.mu.Lock()
		m.orders[uuid] = order
		m.mu.Unlock()
		writeJSON(201, map[string]any{"data": order})

	case r.Method == http.MethodPost && changeMethod.MatchString(path):
		uuid := changeMethod.FindStringSubmatch(path)[1]
		m.mu.Lock()
		order, found := m.orders[uuid]
		m.mu.Unlock()
		if !found {
			writeJSON(404, map[string]any{"message": "Order tidak ditemukan."})
			return
		}
		if order["status"] != "pending" {
			writeJSON(409, map[string]any{"message": "Order sudah tidak pending."})
			return
		}
		var input map[string]any
		_ = json.Unmarshal([]byte(body), &input)
		for _, key := range []string{"qr_string", "payment_no", "payment_name", "app_link"} {
			delete(order, key)
		}
		order["payment_method"] = input["payment_method"]
		order["payment_channel"] = input["payment_channel"]
		switch input["payment_method"] {
		case "qris":
			order["qr_string"] = "NEWQR"
		case "va":
			order["payment_no"] = "8002300000999999"
		case "edc":
			order["app_link"] = "https://erpay.id/payment?data=NEW"
		}
		writeJSON(200, map[string]any{"data": order})

	case r.Method == http.MethodGet && oneOrder.MatchString(path):
		uuid := oneOrder.FindStringSubmatch(path)[1]
		m.mu.Lock()
		order, found := m.orders[uuid]
		m.mu.Unlock()
		if !found {
			writeJSON(400, map[string]any{"message": "record not found"})
			return
		}
		writeJSON(200, map[string]any{"data": order})

	default:
		writeJSON(404, map[string]any{"message": "not found"})
	}
}

func readAll(r *http.Request) string {
	defer r.Body.Close()
	raw, _ := io.ReadAll(r.Body)
	return string(raw)
}
