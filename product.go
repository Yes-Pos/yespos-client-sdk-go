package yespos

import "context"

// InventoryStock adalah stok sebuah produk di satu gudang.
type InventoryStock struct {
	InventoryUUID string  `json:"inventory_uuid"`
	InventoryName string  `json:"inventory_name"`
	CurrentStock  float64 `json:"current_stock"`
}

// ProductVariant adalah varian sebuah produk, dengan harga dan stoknya sendiri.
type ProductVariant struct {
	UUID           string  `json:"uuid"`
	Name           string  `json:"name"`
	Price          Amount  `json:"price"`
	CurrentStock   float64 `json:"current_stock"`
	BarcodeData    string  `json:"barcode_data,omitempty"`
	MainImageToken string  `json:"main_image_token,omitempty"`
	// MainImageURL dibangun oleh API; pakai ini alih-alih menyusunnya dari token.
	MainImageURL string           `json:"main_image_url,omitempty"`
	Inventories  []InventoryStock `json:"inventories,omitempty"`
}

// ProductAddOn adalah tambahan berbayar pada sebuah produk.
type ProductAddOn struct {
	UUID      string `json:"uuid"`
	AddOnName string `json:"add_on_name"`
	Price     Amount `json:"price"`
}

// Product adalah produk yang dijual di sebuah outlet.
type Product struct {
	UUID           string           `json:"uuid"`
	Name           string           `json:"name"`
	Description    string           `json:"description,omitempty"`
	Price          Amount           `json:"price"`
	Unit           string           `json:"unit"`
	Type           string           `json:"type"`
	CurrentStock   float64          `json:"current_stock"`
	BarcodeData    string           `json:"barcode_data,omitempty"`
	MainImageToken string           `json:"main_image_token,omitempty"`
	MainImageURL   string           `json:"main_image_url,omitempty"`
	TrackStock     bool             `json:"track_stock"`
	Variants       []ProductVariant `json:"variants"`
	AddOns         []ProductAddOn   `json:"add_ons,omitempty"`
	Inventories    []InventoryStock `json:"inventories,omitempty"`
}

// ProductDetail adalah satu produk beserta nama kategori yang menaunginya.
type ProductDetail struct {
	Product
	Categories []string `json:"categories"`
}

// ProductCategory adalah satu kategori beserta produk di dalamnya.
type ProductCategory struct {
	CategoryUUID string    `json:"category_uuid"`
	CategoryName string    `json:"category_name"`
	Products     []Product `json:"products"`
}

// IsSoldOut: produk yang melacak stok habis bila stoknya — atau stok semua
// variannya — tinggal nol. Produk yang tidak melacak stok tidak pernah habis.
func (p *Product) IsSoldOut() bool {
	if !p.TrackStock {
		return false
	}
	if len(p.Variants) == 0 {
		return p.CurrentStock <= 0
	}
	for _, variant := range p.Variants {
		if variant.CurrentStock > 0 {
			return false
		}
	}
	return true
}

// PriceRange mengembalikan harga terendah dan tertinggi sebuah produk,
// dihitung dari variannya bila ada.
func (p *Product) PriceRange() (min, max Amount) {
	if len(p.Variants) == 0 {
		return p.Price, p.Price
	}
	min, max = p.Variants[0].Price, p.Variants[0].Price
	for _, variant := range p.Variants[1:] {
		if variant.Price < min {
			min = variant.Price
		}
		if variant.Price > max {
			max = variant.Price
		}
	}
	return min, max
}

// Variant mencari varian berdasarkan uuid.
func (p *Product) Variant(uuid string) (*ProductVariant, bool) {
	for i := range p.Variants {
		if p.Variants[i].UUID == uuid {
			return &p.Variants[i], true
		}
	}
	return nil, false
}

// ProductsService membaca produk yang dijual di sebuah outlet.
type ProductsService struct{ http *HTTPClient }

func productsPath(outletUUID string) string {
	return "/outlets/" + pathEscape(outletUUID) + "/products"
}

// List mengembalikan produk sebuah outlet, dikelompokkan per kategori.
func (s *ProductsService) List(ctx context.Context, outletUUID string) ([]ProductCategory, error) {
	var categories []ProductCategory
	if err := s.http.Get(ctx, productsPath(outletUUID), nil, &categories); err != nil {
		return nil, err
	}
	return categories, nil
}

// Get mengembalikan satu produk sebuah outlet beserta nama kategorinya.
func (s *ProductsService) Get(ctx context.Context, outletUUID, productUUID string) (*ProductDetail, error) {
	var product ProductDetail
	if err := s.http.Get(ctx, productsPath(outletUUID)+"/"+pathEscape(productUUID), nil, &product); err != nil {
		return nil, err
	}
	return &product, nil
}

// Map mengembalikan seluruh produk sebuah outlet dengan kunci uuid. Satu produk
// bisa berada di beberapa kategori; di sini ia hanya muncul sekali.
func (s *ProductsService) Map(ctx context.Context, outletUUID string) (map[string]Product, error) {
	categories, err := s.List(ctx, outletUUID)
	if err != nil {
		return nil, err
	}
	products := make(map[string]Product)
	for _, category := range categories {
		for _, product := range category.Products {
			products[product.UUID] = product
		}
	}
	return products, nil
}
