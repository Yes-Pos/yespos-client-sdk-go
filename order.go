package yespos

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// OrderItem adalah satu baris pada order.
type OrderItem struct {
	ProductName string `json:"product_name"`
	Quantity    int    `json:"quantity"`
	Price       Amount `json:"price"`
	Total       Amount `json:"total"`
}

// SplitItem membagi pembayaran ke beberapa penerima.
type SplitItem struct {
	VA     string `json:"va"`
	Amount Amount `json:"amount"`
	Status string `json:"status,omitempty"`
}

// Customer adalah pembeli sebuah order.
type Customer struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

// Order adalah order beserta status dan rincian pembayarannya.
type Order struct {
	UUID           string `json:"uuid"`
	TotalPayment   Amount `json:"total_payment"`
	PaymentMethod  string `json:"payment_method"`
	PaymentChannel string `json:"payment_channel,omitempty"`
	// Status mula-mula "pending", lalu "<status>_<settlement>",
	// misalnya "berhasil_settled".
	Status string `json:"status"`
	// QRString diisi untuk QRIS; render menjadi kode QR.
	QRString string `json:"qr_string,omitempty"`
	// AppLink diisi untuk EDC; buka di perangkat EDC.
	AppLink string `json:"app_link,omitempty"`
	// Nomor virtual account, nama pemilik, dan batas waktu bayar.
	PaymentNo   string `json:"payment_no,omitempty"`
	PaymentName string `json:"payment_name,omitempty"`
	ExpiredAt   string `json:"expired_at,omitempty"`

	Items []OrderItem `json:"items"`
	// Customer kosong untuk order lama, sebelum YesPOS menyimpan pembeli.
	Customer        *Customer   `json:"customer,omitempty"`
	Split           []SplitItem `json:"split,omitempty"`
	NotifyURL       string      `json:"notify_url,omitempty"`
	ReferenceIpaymu string      `json:"reference_ipaymu,omitempty"`
	CreatedAt       string      `json:"created_at"`
}

// CreateOrderInput adalah body POST /api/client/orders.
type CreateOrderInput struct {
	Customer
	TotalPayment  Amount `json:"total_payment"`
	PaymentMethod string `json:"payment_method"`
	// PaymentChannel wajib bila metode punya lebih dari satu channel aktif,
	// misalnya bank virtual account.
	PaymentChannel string      `json:"payment_channel,omitempty"`
	Items          []OrderItem `json:"items"`
	NotifyURL      string      `json:"notify_url,omitempty"`
	Split          []SplitItem `json:"split,omitempty"`
	AppCode        string      `json:"app_code,omitempty"`
	TerminalCode   string      `json:"terminal_code,omitempty"`
	MerchantCode   string      `json:"merchant_code,omitempty"`
}

// ChangeOrderPaymentInput adalah body POST /api/client/orders/:uuid/payment-method.
type ChangeOrderPaymentInput struct {
	PaymentMethod  string `json:"payment_method"`
	PaymentChannel string `json:"payment_channel,omitempty"`
	// Nama, email, dan telepon hanya untuk order yang belum punya pembeli;
	// keduanya lalu ikut disimpan bersama order.
	Name         string `json:"name,omitempty"`
	Email        string `json:"email,omitempty"`
	Phone        string `json:"phone,omitempty"`
	AppCode      string `json:"app_code,omitempty"`
	TerminalCode string `json:"terminal_code,omitempty"`
	MerchantCode string `json:"merchant_code,omitempty"`
}

// OrderListParams adalah paginasi untuk GET /api/client/orders.
type OrderListParams struct {
	Page  int
	Limit int
}

// OrderList adalah satu halaman order.
type OrderList struct {
	Data  []Order `json:"data"`
	Total int     `json:"total"`
	Page  int     `json:"page"`
	Limit int     `json:"limit"`
}

// OrderState adalah status order yang sudah disederhanakan.
type OrderState string

const (
	OrderStatePending OrderState = "pending"
	OrderStatePaid    OrderState = "paid"
	OrderStateFailed  OrderState = "failed"
)

// ParseOrderState membaca status order. Status YesPOS berupa "pending" atau
// "<status>_<settlement>", misalnya "berhasil_settled".
func ParseOrderState(status string) OrderState {
	value := strings.ToLower(status)
	switch {
	case strings.HasPrefix(value, "pending"):
		return OrderStatePending
	case strings.HasPrefix(value, "berhasil"), strings.HasPrefix(value, "success"):
		return OrderStatePaid
	default:
		return OrderStateFailed
	}
}

// State mengembalikan status order yang sudah disederhanakan.
func (o *Order) State() OrderState { return ParseOrderState(o.Status) }

// IsPending bernilai true selama order masih menunggu dibayar.
func (o *Order) IsPending() bool { return o.State() == OrderStatePending }

// IsPaid bernilai true begitu pembayarannya masuk.
func (o *Order) IsPaid() bool { return o.State() == OrderStatePaid }

// OrdersService mengelola order milik client.
type OrdersService struct{ http *HTTPClient }

// Create menempatkan order dan meminta iPaymu atau TSM menagihnya. Responsnya
// membawa apa yang dibutuhkan pembeli: QRString (QRIS), PaymentNo dan ExpiredAt
// (virtual account), atau AppLink (EDC).
func (s *OrdersService) Create(ctx context.Context, input CreateOrderInput) (*Order, error) {
	var order Order
	if err := s.http.Post(ctx, "/orders", input, &order); err != nil {
		return nil, err
	}
	return &order, nil
}

// Get mengembalikan satu order beserta status dan rincian pembayarannya.
func (s *OrdersService) Get(ctx context.Context, uuid string) (*Order, error) {
	var order Order
	if err := s.http.Get(ctx, "/orders/"+pathEscape(uuid), nil, &order); err != nil {
		return nil, err
	}
	return &order, nil
}

// List mengembalikan order milik client, terbaru lebih dulu.
func (s *OrdersService) List(ctx context.Context, params OrderListParams) (*OrderList, error) {
	query := url.Values{}
	if params.Page > 0 {
		query.Set("page", strconv.Itoa(params.Page))
	}
	if params.Limit > 0 {
		query.Set("limit", strconv.Itoa(params.Limit))
	}
	var list OrderList
	if err := s.http.Get(ctx, "/orders", query, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

// ChangePaymentMethod memindahkan order pending ke metode lain yang aktif,
// dengan total, item, split, dan pembeli yang sama. Mengirim metode yang sedang
// dipakai akan memperbarui pembayarannya, misalnya virtual account kedaluwarsa.
func (s *OrdersService) ChangePaymentMethod(ctx context.Context, uuid string, input ChangeOrderPaymentInput) (*Order, error) {
	var order Order
	if err := s.http.Post(ctx, "/orders/"+pathEscape(uuid)+"/payment-method", input, &order); err != nil {
		return nil, err
	}
	return &order, nil
}
