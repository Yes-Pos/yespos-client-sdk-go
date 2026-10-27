# Contoh pemakaian

Dua program yang bisa langsung dijalankan. Keduanya memakai environment variable yang sama:

```bash
export YESPOS_API_BASE_URL="https://api.yespos.id"
export YESPOS_CLIENT_KEY="…"
export YESPOS_CLIENT_SECRET="…"
export YESPOS_OUTLET_UUID="…"                      # opsional, kosong = outlet pertama
export YESPOS_NOTIFY_URL="https://…/api/notify"    # opsional, callback status order
```

## `quickstart` — CLI

Menampilkan outlet, katalog, dan metode pembayaran yang aktif; bisa sekalian membuat satu order percobaan.

```bash
go run ./examples/quickstart                                        # lihat katalog
go run ./examples/quickstart -order p1 -qty 2 -payment va:bca       # buat order
go run ./examples/quickstart -order p2 -variant v1 -payment qris:qris
go run ./examples/quickstart -status <uuid-order>                   # cek status
```

Flag lain: `-outlet`, `-timeout`. Jalankan dengan `-h` untuk daftar lengkapnya.

## `storefront` — toko online

Server HTTP dengan katalog, keranjang, checkout, dan penerima callback.

```bash
go run ./examples/storefront        # lalu buka http://localhost:8080
ADDR=:3000 go run ./examples/storefront
```

| Endpoint | Guna |
|---|---|
| `GET /` | Halaman toko (HTML + JS, tanpa dependency) |
| `GET /api/catalog` | Outlet dan produknya |
| `GET /api/payment-methods` | Metode aktif; EDC muncul hanya bila terminalnya dikonfigurasi |
| `POST /api/checkout` | Membuat order dari keranjang |
| `GET /api/orders/{uuid}` | Status dan instruksi bayar sebuah order |
| `POST /api/notify` | Callback status order dari YesPOS |

### Pola yang dipakai

**SDK hanya hidup di server.** Browser memanggil endpoint JSON milik aplikasi sendiri, jadi `YESPOS_CLIENT_SECRET` tidak pernah ikut terkirim ke mana pun selain header signature.

**Harga tidak pernah datang dari browser.** `POST /api/checkout` hanya menerima `product_uuid`, `variant_uuid`, dan `quantity`; `shop.PlaceOrder` mengambil harganya dari API lalu menghitung total sendiri. Angka total yang tampil di halaman cuma perkiraan untuk pembeli.

**Status akhir datang lewat callback, bukan polling.** `POST /api/notify` mencatat perubahan status, lalu membaca ulang order lewat SDK sebagai sumber kebenaran — isi body callback tidak dipercaya begitu saja. Di halaman, status hanya dicek saat pembeli menekan tombol.

**Error dari YesPOS diteruskan apa adanya**, kecuali kegagalan kredensial: itu masalah server, jadi pembeli hanya melihat pesan umum sementara detailnya masuk ke log.

## Menguji tanpa kredensial asli

Kedua contoh ini berbicara dengan API sungguhan. Untuk mencobanya tanpa akun, arahkan `YESPOS_API_BASE_URL` ke server tiruan — `mock_test.go` di akar repo berisi satu yang memverifikasi signature persis seperti backend.
