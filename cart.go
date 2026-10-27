package yespos

// CartLine adalah satu baris keranjang yang dikirim storefront saat checkout:
// tanpa harga, SDK yang mencarinya ke API.
type CartLine struct {
	ProductUUID string
	VariantUUID string
	Quantity    int
}
