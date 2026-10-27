package yespos

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultTimeout adalah batas waktu satu request bila tidak diatur.
	DefaultTimeout = 15 * time.Second
	// DefaultLanguage adalah bahasa pesan API bila tidak diatur.
	DefaultLanguage = "id"
)

// ClientOptions adalah konfigurasi untuk membuat [Client] atau [HTTPClient].
type ClientOptions struct {
	// BaseURL adalah alamat YesPOS API, misalnya "https://api.yespos.id".
	// Garis miring di akhir boleh ada, akan dibuang.
	BaseURL string
	// Key adalah client credential key, dikirim sebagai header "va".
	Key string
	// Secret menandatangani setiap request dan harus tetap di server.
	Secret string
	// Timeout satu request; default [DefaultTimeout].
	Timeout time.Duration
	// Language adalah bahasa pesan API; default [DefaultLanguage].
	Language string
	// HTTPClient opsional, misalnya yang memakai proxy atau retry.
	HTTPClient *http.Client
}

// Sign menandatangani sebuah request persis seperti yang dicek
// ClientAuthMiddleware di YesPOS:
//
//	HMAC-SHA256(secret, "METHOD:key:sha256(body):secret")
//
// Body kosong ditandatangani sebagai "{}".
func Sign(method, key, secret, body string) string {
	if body == "" {
		body = "{}"
	}
	bodyHash := sha256.Sum256([]byte(body))
	message := fmt.Sprintf("%s:%s:%s:%s", strings.ToUpper(method), key, hex.EncodeToString(bodyHash[:]), secret)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// HTTPClient adalah transport untuk /api/client: menandatangani setiap request
// dan membuka amplop {message, data} dari respons.
//
// Dipakai oleh resource; panggil langsung hanya untuk endpoint yang belum
// dibungkus SDK.
type HTTPClient struct {
	baseURL  string
	key      string
	secret   string
	language string
	timeout  time.Duration
	client   *http.Client
}

// NewHTTPClient membuat transport dari opts.
func NewHTTPClient(opts ClientOptions) (*HTTPClient, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" || opts.Key == "" || opts.Secret == "" {
		return nil, &Error{Status: 500, Message: "baseUrl, key dan secret wajib diisi saat membuat Client."}
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	language := opts.Language
	if language == "" {
		language = DefaultLanguage
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	return &HTTPClient{
		baseURL:  baseURL,
		key:      opts.Key,
		secret:   opts.Secret,
		language: language,
		timeout:  timeout,
		client:   client,
	}, nil
}

// MediaURL mengembalikan URL publik sebuah file yang diunggah;
// GET /media/:token tidak butuh login.
func (c *HTTPClient) MediaURL(token string) string {
	return c.baseURL + "/media/" + url.PathEscape(token)
}

// Get memanggil endpoint dengan method GET dan membaca data ke out.
func (c *HTTPClient) Get(ctx context.Context, path string, query url.Values, out any) error {
	return c.Do(ctx, http.MethodGet, path, nil, query, out)
}

// Post memanggil endpoint dengan method POST dan membaca data ke out.
func (c *HTTPClient) Post(ctx context.Context, path string, body, out any) error {
	return c.Do(ctx, http.MethodPost, path, body, nil, out)
}

// Do mengirim satu request yang sudah ditandatangani lalu membuka amplopnya.
// out boleh nil bila responsnya tidak dipakai.
func (c *HTTPClient) Do(ctx context.Context, method, path string, body any, query url.Values, out any) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// Body diserialisasi di sini supaya tanda tangan menutupi persis apa yang dikirim.
	payload := ""
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return &Error{Status: 500, Message: "Gagal menyusun body permintaan: " + err.Error()}
		}
		payload = string(raw)
	}
	signature := Sign(method, c.key, c.secret, payload)

	endpoint := c.baseURL + "/api/client" + path
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var reader io.Reader
	if payload != "" {
		reader = bytes.NewReader([]byte(payload))
	}
	request, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), endpoint, reader)
	if err != nil {
		return &Error{Status: 500, Message: "Permintaan ke YesPOS tidak dapat dibuat: " + err.Error()}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept-Language", c.language)
	request.Header.Set("va", c.key)
	request.Header.Set("signature", signature)
	request.Header.Set("timestamp", strconv.FormatInt(time.Now().Unix(), 10))

	response, err := c.client.Do(request)
	if err != nil {
		return &Error{Status: 0, Message: connectionMessage(err)}
	}
	defer response.Body.Close()

	text, err := io.ReadAll(response.Body)
	if err != nil {
		return &Error{Status: 0, Message: connectionMessage(err)}
	}

	if response.StatusCode < 200 || response.StatusCode > 299 {
		var failure errorBody
		_ = json.Unmarshal(text, &failure)
		message := string(failure.Message)
		if message == "" {
			message = fmt.Sprintf("Permintaan gagal (%d).", response.StatusCode)
		}
		return &Error{Status: response.StatusCode, Message: message, Details: failure.Details}
	}

	if out == nil {
		return nil
	}
	var envelope struct {
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if len(bytes.TrimSpace(text)) > 0 {
		if err := json.Unmarshal(text, &envelope); err != nil {
			return &Error{Status: response.StatusCode, Message: "Respons YesPOS tidak dapat dibaca: " + err.Error()}
		}
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return &Error{Status: response.StatusCode, Message: "Respons YesPOS tidak dapat dibaca: " + err.Error()}
	}
	return nil
}

// connectionMessage membedakan kehabisan waktu dari gagal terhubung.
func connectionMessage(err error) string {
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return "Permintaan ke YesPOS melebihi batas waktu."
	}
	return "Tidak dapat terhubung ke server YesPOS."
}
