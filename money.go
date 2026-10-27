package yespos

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Amount adalah nominal rupiah dalam satuan penuh (tanpa sen).
//
// API kadang mengirim angka bulat, angka desimal, atau string; semuanya
// diterima dan dibulatkan ke rupiah terdekat. Saat dikirim kembali, nilainya
// selalu berupa angka JSON biasa.
type Amount int64

func (a Amount) MarshalJSON() ([]byte, error) {
	return strconv.AppendInt(nil, int64(a), 10), nil
}

func (a *Amount) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if text == "" || text == "null" {
		*a = 0
		return nil
	}
	if unquoted, err := strconv.Unquote(text); err == nil {
		text = strings.TrimSpace(unquoted)
	}
	if text == "" {
		*a = 0
		return nil
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return fmt.Errorf("yespos: nilai uang tidak dikenali: %s", data)
	}
	*a = Amount(math.Round(value))
	return nil
}

func (a Amount) String() string { return strconv.FormatInt(int64(a), 10) }
