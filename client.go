package yespos

import "os"

// Client adalah akses ke YesPOS Client API (/api/client/*).
//
// Setiap request ditandatangani dengan client secret, jadi ini harus berjalan
// di server: jangan pernah mengirim secret ke browser.
//
//	client, err := yespos.NewClient(yespos.ClientOptions{BaseURL: baseURL, Key: key, Secret: secret})
//	outlets, err := client.Outlets.List(ctx)
//	products, err := client.Products.List(ctx, outlets[0].UUID)
type Client struct {
	HTTP           *HTTPClient
	Outlets        *OutletsService
	Products       *ProductsService
	PaymentMethods *PaymentMethodsService
	Orders         *OrdersService
}

// NewClient membuat client dari opts.
func NewClient(opts ClientOptions) (*Client, error) {
	http, err := NewHTTPClient(opts)
	if err != nil {
		return nil, err
	}
	return &Client{
		HTTP:           http,
		Outlets:        &OutletsService{http: http},
		Products:       &ProductsService{http: http},
		PaymentMethods: &PaymentMethodsService{http: http},
		Orders:         &OrdersService{http: http},
	}, nil
}

// MediaURL mengembalikan URL publik sebuah gambar yang diunggah;
// GET /media/:token tidak butuh login.
func (c *Client) MediaURL(token string) string { return c.HTTP.MediaURL(token) }

// EnvLookup membaca satu variabel environment. Lihat [MapEnv] untuk memakai
// map biasa, misalnya di dalam test.
type EnvLookup func(key string) string

// MapEnv membuat [EnvLookup] dari sebuah map.
func MapEnv(values map[string]string) EnvLookup {
	return func(key string) string { return values[key] }
}

// lookupOrOS memilih env pertama yang diberikan, atau os.Getenv bila tak ada.
func lookupOrOS(env []EnvLookup) EnvLookup {
	if len(env) > 0 && env[0] != nil {
		return env[0]
	}
	return os.Getenv
}

// ClientFromEnv membuat client dari variabel environment yang biasa dipakai:
// YESPOS_API_BASE_URL, YESPOS_CLIENT_KEY, YESPOS_CLIENT_SECRET.
func ClientFromEnv(env ...EnvLookup) (*Client, error) {
	get := lookupOrOS(env)
	return NewClient(ClientOptions{
		BaseURL: get("YESPOS_API_BASE_URL"),
		Key:     get("YESPOS_CLIENT_KEY"),
		Secret:  get("YESPOS_CLIENT_SECRET"),
	})
}
