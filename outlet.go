package yespos

import (
	"context"
	"net/url"
)

// Outlet adalah outlet milik pemilik client (GET /api/client/outlets).
type Outlet struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
	Contact string `json:"contact,omitempty"`
	Type    string `json:"type,omitempty"`
}

// OutletsService membaca outlet milik pemilik client.
type OutletsService struct{ http *HTTPClient }

// List mengembalikan seluruh outlet milik pemilik client.
func (s *OutletsService) List(ctx context.Context) ([]Outlet, error) {
	var outlets []Outlet
	if err := s.http.Get(ctx, "/outlets", nil, &outlets); err != nil {
		return nil, err
	}
	return outlets, nil
}

// pathEscape membungkus segmen path agar aman dipakai di URL.
func pathEscape(segment string) string { return url.PathEscape(segment) }
