// Package yespos adalah SDK Go untuk YesPOS Client API (/api/client/*):
// outlet, produk, metode pembayaran, dan order dengan QRIS, Virtual Account,
// atau EDC.
//
// Setiap request ditandatangani dengan client secret, jadi SDK ini hanya untuk
// dipakai di server. Jangan pernah menaruh secret di aplikasi yang dibagikan ke
// pengguna akhir.
//
// Ada dua lapisan:
//
//   - [Client] — akses mentah ke endpoint, tanpa aturan tambahan.
//   - [Shop]   — logika toko: memilih outlet, menyembunyikan EDC bila tak
//     dikonfigurasi, dan menghitung harga order dari API.
package yespos
