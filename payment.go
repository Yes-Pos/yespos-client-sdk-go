package yespos

import "context"

// Kode metode pembayaran yang dikenal YesPOS.
const (
	PaymentMethodQRIS = "qris"
	PaymentMethodVA   = "va"
	PaymentMethodEDC  = "edc"
)

// PaymentMethod adalah metode pembayaran yang diaktifkan pemilik client
// (GET /api/client/payment-method).
type PaymentMethod struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Issuer         string `json:"issuer"`
	PaymentMethod  string `json:"payment_method"`
	PaymentChannel string `json:"payment_channel"`
	// RequiredFields adalah field order tambahan yang diminta metode ini,
	// misalnya kode terminal EDC.
	RequiredFields []string `json:"required_fields"`
}

// Value adalah bentuk "<method>:<channel>" yang dipakai storefront untuk
// menamai satu pilihan pembayaran, misalnya "va:bca".
func (m PaymentMethod) Value() string {
	return m.PaymentMethod + ":" + m.PaymentChannel
}

// PaymentValue adalah bentuk fungsi dari [PaymentMethod.Value].
func PaymentValue(m PaymentMethod) string { return m.Value() }

// EdcTerminal adalah terminal EDC yang menagih sebuah order.
type EdcTerminal struct {
	AppCode      string `json:"app_code"`
	TerminalCode string `json:"terminal_code"`
	MerchantCode string `json:"merchant_code"`
}

// PaymentMethodsService membaca metode pembayaran yang boleh dipakai order.
type PaymentMethodsService struct{ http *HTTPClient }

// List mengembalikan metode yang diaktifkan pemilik client: QRIS, virtual
// account per bank, dan EDC. Metode di luar daftar ini akan ditolak saat
// membuat order.
func (s *PaymentMethodsService) List(ctx context.Context) ([]PaymentMethod, error) {
	var methods []PaymentMethod
	if err := s.http.Get(ctx, "/payment-method", nil, &methods); err != nil {
		return nil, err
	}
	return methods, nil
}
