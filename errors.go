package yespos

import (
	"encoding/json"
	"errors"
	"strings"
)

// Error adalah kegagalan yang dijawab YesPOS API, atau kegagalan menghubunginya.
//
// Status berisi status HTTP, atau 0 bila request tidak pernah mendapat respons.
// Details membawa field "details" dari API, misalnya daftar produk yang membuat
// sebuah pengajuan verifikasi ditolak.
type Error struct {
	Status  int
	Message string
	Details json.RawMessage
}

func (e *Error) Error() string { return e.Message }

// IsAuthError bernilai true bila kredensial ditolak atau toko salah konfigurasi.
func (e *Error) IsAuthError() bool { return e.Status == 401 || e.Status == 403 }

// IsNetworkError bernilai true bila YesPOS sama sekali tidak terjangkau.
func (e *Error) IsNetworkError() bool { return e.Status == 0 || e.Status == 502 }

// DecodeDetails membaca field "details" dari API ke dalam v.
// Mengembalikan nil bila tidak ada details.
func (e *Error) DecodeDetails(v any) error {
	if len(e.Details) == 0 || string(e.Details) == "null" {
		return nil
	}
	return json.Unmarshal(e.Details, v)
}

// AsError menyempitkan error apa pun menjadi [*Error] bila berasal dari SDK ini.
func AsError(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// IsError bernilai true bila err berasal dari SDK ini.
func IsError(err error) bool {
	_, ok := AsError(err)
	return ok
}

// errorBody adalah bentuk body yang dikirim API saat menolak sebuah request.
type errorBody struct {
	Message apiMessage      `json:"message"`
	Details json.RawMessage `json:"details"`
}

// apiMessage menerima pesan berupa string maupun daftar pesan validasi.
type apiMessage string

func (m *apiMessage) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*m = apiMessage(single)
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err == nil {
		*m = apiMessage(strings.Join(many, " "))
		return nil
	}
	*m = ""
	return nil
}
