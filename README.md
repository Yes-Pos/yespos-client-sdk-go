# yespos-client-sdk-go

SDK Go untuk **YesPOS Client API** (`/api/client/*`): outlet, produk, metode pembayaran, dan order dengan **QRIS**, **Virtual Account**, atau **EDC**.

- **Tanpa dependency.** Hanya memakai standard library.
- **Go 1.22+.**
- **`context.Context` di setiap panggilan**, jadi timeout dan pembatalan ikut aturanmu.

> ⚠️ **Hanya untuk server.** Setiap request ditandatangani dengan client secret. Jangan pernah menanam secret di aplikasi mobile atau desktop yang dibagikan ke pengguna — panggil dari backend.

## Instalasi

```bash
go get github.com/Yes-Pos/yespos-client-sdk-go
```

```go
import yespos "github.com/Yes-Pos/yespos-client-sdk-go"
```

## Mulai cepat

```go
shop, err := yespos.NewShopWithOptions(
	yespos.ClientOptions{
		BaseURL: os.Getenv("YESPOS_API_BASE_URL"),
		Key:     os.Getenv("YESPOS_CLIENT_KEY"),
		Secret:  os.Getenv("YESPOS_CLIENT_SECRET"),
	},
	yespos.ShopOptions{OutletUUID: os.Getenv("YESPOS_OUTLET_UUID")},
)
if err != nil {
	log.Fatal(err)
}

ctx := context.Background()
categories, err := shop.Products(ctx)
methods, err := shop.PaymentMethods(ctx)

order, err := shop.PlaceOrder(ctx,
	[]yespos.CartLine{{ProductUUID: "abc-123", Quantity: 2}},          // tanpa harga
	yespos.Customer{Name: "Budi", Email: "budi@example.com", Phone: "08123456789"},
	"va:bca",                                                           // "<method>:<channel>"
)

order.PaymentNo  // nomor VA yang dibayar pembeli
order.ExpiredAt  // batas waktu bayar
```

Atau langsung dari environment variable:

```go
shop, err := yespos.ShopFromEnv(yespos.ShopOptions{}) // membaca YESPOS_* dari os.Getenv
```

## Dua lapisan

| Lapisan | Untuk apa |
|---|---|
| `Client` | Akses mentah ke endpoint. Tidak ada aturan tambahan. |
| `Shop` | Logika toko: pilih outlet, sembunyikan EDC bila tak dikonfigurasi, dan **hitung harga dari API**. |

### `Shop` — dipakai storefront

```go
shop := yespos.NewShop(client, yespos.ShopOptions{
	OutletUUID: "uuid-outlet",                   // kosong = outlet pertama milik pemilik client
	NotifyURL:  "https://…/callback",            // callback status order
	EDC:        &yespos.EdcTerminal{AppCode: "YESPOS", TerminalCode: "T1", MerchantCode: "M1"}, // nil = EDC disembunyikan
	Messages:   yespos.ShopMessages{MethodUnavailable: "Metode pembayaran tidak tersedia."},    // untuk terjemahan
})

shop.Outlet(ctx)                                   // *Outlet
shop.Products(ctx)                                 // []ProductCategory
shop.Product(ctx, "uuid-produk")                   // *ProductDetail
shop.PaymentMethods(ctx)                           // []PaymentMethod
shop.PriceCart(ctx, lines)                         // []OrderItem dengan harga dari API
shop.PlaceOrder(ctx, lines, buyer, "qris:qris")    // *Order
shop.Order(ctx, uuid)                              // *Order
shop.ChangePayment(ctx, uuid, "va:bca", nil)       // order pending → metode lain
shop.MediaURL(token)                               // URL gambar publik
```

**Harga selalu diambil dari API.** `PlaceOrder` hanya menerima `ProductUUID`, `VariantUUID`, dan `Quantity`; totalnya dihitung ulang dari harga yang dibaca API. Harga yang dikirim dari browser tidak dipercaya.

`ShopMessages` yang field-nya kosong memakai pesan bawaan, jadi cukup isi yang ingin diterjemahkan.

### `Client` — akses langsung

```go
client, err := yespos.NewClient(yespos.ClientOptions{
	BaseURL: baseURL,
	Key:     key,
	Secret:  secret,
	Timeout: 15 * time.Second,
	Language: "id",
})

client.Outlets.List(ctx)
client.Products.List(ctx, outletUUID)
client.Products.Get(ctx, outletUUID, productUUID)
client.Products.Map(ctx, outletUUID)               // map[string]Product
client.PaymentMethods.List(ctx)
client.Orders.Create(ctx, input)
client.Orders.Get(ctx, uuid)
client.Orders.List(ctx, yespos.OrderListParams{Page: 1, Limit: 10})
client.Orders.ChangePaymentMethod(ctx, uuid, input)
client.MediaURL(token)
```

Endpoint yang belum dibungkus bisa dipanggil lewat transport-nya:

```go
var hasil Something
err := client.HTTP.Get(ctx, "/endpoint-baru", nil, &hasil)
err := client.HTTP.Post(ctx, "/endpoint-baru", body, &hasil)
```

## Metode pembayaran

`GET /api/client/payment-method` hanya mengembalikan metode yang **diaktifkan oleh pemilik client** di dashboard YesPOS. Metode di luar daftar itu akan ditolak saat membuat order.

Satu pilihan ditulis `"<method>:<channel>"`:

| Nilai | Arti | Field di response order |
|---|---|---|
| `qris:qris` | QRIS | `QRString` → render jadi QR |
| `va:bca`, `va:bri`, … | Virtual account per bank | `PaymentNo`, `PaymentName`, `ExpiredAt` |
| `edc:linkpayment` | Kartu lewat EDC | `AppLink` → buka di perangkat EDC |

```go
methods, _ := shop.PaymentMethods(ctx)
for _, method := range methods {
	fmt.Println(method.Value(), method.Name) // "va:bca" "BCA"
}
```

## Status order

```go
yespos.ParseOrderState("pending")          // yespos.OrderStatePending
yespos.ParseOrderState("berhasil_settled") // yespos.OrderStatePaid
yespos.ParseOrderState("expired")          // yespos.OrderStateFailed

order.State()
order.IsPending()
order.IsPaid()
```

Status akhir dikirim YesPOS ke `NotifyURL`. Untuk menampilkannya di halaman, panggil `shop.Order(ctx, uuid)` saat pembeli menekan tombol cek status — hindari polling berkala.

## Nominal uang

Harga dan total memakai tipe `yespos.Amount`, yaitu `int64` rupiah penuh. API kadang mengirim angka bulat, angka desimal, atau string; semuanya diterima dan dibulatkan, lalu dikirim kembali sebagai angka JSON biasa.

```go
total := order.TotalPayment      // yespos.Amount
int64(total)                     // 50000
```

## Error

Semua kegagalan dikembalikan sebagai `*yespos.Error`.

```go
order, err := shop.PlaceOrder(ctx, lines, buyer, "va:bca")
var sdkError *yespos.Error
if errors.As(err, &sdkError) {
	sdkError.Status             // status HTTP; 0 bila server tak terjangkau
	sdkError.Message            // pesan dari API, sudah digabung bila berupa daftar
	sdkError.DecodeDetails(&v)  // field `details` dari API, bila ada
	sdkError.IsNetworkError()   // true bila tidak terhubung / timeout
	sdkError.IsAuthError()      // true untuk 401/403
}
```

`yespos.AsError(err)` dan `yespos.IsError(err)` tersedia sebagai pintasan.

Beberapa yang sering muncul:

| Status | Arti |
|---|---|
| 400 | Metode pembayaran tidak tersedia, atau `payment_channel` wajib diisi |
| 404 | Outlet, produk, atau order tidak ditemukan |
| 409 | Produk di keranjang sudah tidak dijual, atau order sudah tidak pending |
| 401 | Signature ditolak — periksa `Key` dan `Secret` |

## Konfigurasi

```go
yespos.ClientOptions{
	BaseURL:    "https://api.yespos.id", // tanpa / di akhir (tetap aman bila ada)
	Key:        "…",                     // header `va`
	Secret:     "…",                     // penandatangan request, simpan di server
	Timeout:    15 * time.Second,         // default 15 detik
	Language:   "id",                     // bahasa pesan API, default "id"
	HTTPClient: custom,                   // opsional: *http.Client sendiri (proxy, retry, dll.)
}
```

Variabel environment yang dibaca `ShopFromEnv()` / `ClientFromEnv()`:

```
YESPOS_API_BASE_URL, YESPOS_CLIENT_KEY, YESPOS_CLIENT_SECRET,
YESPOS_OUTLET_UUID, YESPOS_NOTIFY_URL,
YESPOS_EDC_APP_CODE, YESPOS_EDC_TERMINAL_CODE, YESPOS_EDC_MERCHANT_CODE
```

Keduanya menerima sumber environment lain, berguna di dalam test:

```go
shop, err := yespos.ShopFromEnv(yespos.ShopOptions{}, yespos.MapEnv(map[string]string{…}))
```

Field `ShopOptions` yang sudah terisi menimpa nilai dari environment.

## Cara kerja signature

SDK menandatangani setiap request seperti yang dicek `ClientAuthMiddleware` di YesPOS:

```
signature = HMAC-SHA256(secret, "METHOD:key:sha256(body || "{}"):secret")
```

Lalu dikirim sebagai header `va`, `signature`, dan `timestamp`. Fungsinya diekspor sebagai `yespos.Sign()` bila diperlukan di luar SDK.

## Contoh pemakaian

Dua program siap jalan ada di repo tersendiri, [`example-go`](https://github.com/Yes-Pos/example-go):

```bash
git clone https://github.com/Yes-Pos/example-go && cd example-go
go run ./cmd/quickstart     # CLI: katalog, metode pembayaran, buat order
go run ./cmd/storefront     # toko online di http://localhost:8080
```

`storefront` memperagakan pola yang dipakai storefront sungguhan: SDK hanya di server, browser memanggil endpoint JSON milik aplikasi sendiri, dan harga selalu dihitung ulang dari API.

Untuk Laravel ada [`example-laravel`](https://github.com/Yes-Pos/example-laravel).

## SDK lain

Versi TypeScript-nya ada di [`@yespos/client`](https://github.com/Yes-Pos/yespos-client-sdk) dengan bentuk API yang sama.

## Pengembangan

```bash
go test ./...            # termasuk mock yang memverifikasi signature
go test -race -cover ./...
go vet ./...
gofmt -l .
```

## Rilis

Modul Go diterbitkan lewat tag git, bukan lewat registry.

```bash
git tag v0.1.0
git push --follow-tags
```

Workflow `release.yml` menjalankan vet + test, membuat GitHub Release, lalu meminta `proxy.golang.org` mengambil versi tersebut agar `go get` langsung menemukannya dan dokumentasi terbit di pkg.go.dev.

Untuk rilis major v2 ke atas, module path di `go.mod` harus ikut berakhiran `/v2` — workflow akan menolak tag yang tidak cocok.

## Lisensi

MIT
