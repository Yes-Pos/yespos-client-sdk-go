package yespos_test

import (
	"context"
	"errors"
	"fmt"
	"log"

	yespos "github.com/Yes-Pos/yespos-client-sdk-go"
)

// Storefront: Shop mengambil harga dari API, jadi keranjang dari browser tidak
// bisa mengubah tagihan pembeli.
func ExampleShop_PlaceOrder() {
	shop, err := yespos.ShopFromEnv(yespos.ShopOptions{})
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	order, err := shop.PlaceOrder(ctx,
		[]yespos.CartLine{{ProductUUID: "abc-123", Quantity: 2}}, // tanpa harga
		yespos.Customer{Name: "Budi", Email: "budi@example.com", Phone: "08123456789"},
		"va:bca", // "<method>:<channel>"
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(order.PaymentNo) // nomor VA yang dibayar pembeli
	fmt.Println(order.ExpiredAt) // batas waktu bayar
}

// Akses langsung ke endpoint, tanpa aturan tambahan.
func ExampleClient() {
	client, err := yespos.NewClient(yespos.ClientOptions{
		BaseURL: "https://api.yespos.id",
		Key:     "…",
		Secret:  "…",
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	outlets, err := client.Outlets.List(ctx)
	if err != nil {
		log.Fatal(err)
	}
	categories, err := client.Products.List(ctx, outlets[0].UUID)
	if err != nil {
		log.Fatal(err)
	}

	for _, category := range categories {
		for _, product := range category.Products {
			min, max := product.PriceRange()
			fmt.Printf("%s %d-%d habis=%v\n", product.Name, min, max, product.IsSoldOut())
		}
	}
}

// Semua kegagalan dikembalikan sebagai *yespos.Error.
func ExampleAsError() {
	shop, err := yespos.ShopFromEnv(yespos.ShopOptions{})
	if err != nil {
		log.Fatal(err)
	}

	_, err = shop.PlaceOrder(context.Background(), nil, yespos.Customer{}, "va:bca")
	var sdkError *yespos.Error
	if errors.As(err, &sdkError) {
		fmt.Println(sdkError.Status)           // status HTTP; 0 bila server tak terjangkau
		fmt.Println(sdkError.Message)          // pesan dari API, sudah digabung bila berupa daftar
		fmt.Println(sdkError.IsNetworkError()) // true bila tidak terhubung / timeout
		fmt.Println(sdkError.IsAuthError())    // true untuk 401/403
	}
}
