# Laporan verifikasi

Tanggal: 20 Oktober 2026. Source dan pengujian pada commit `787c6f4` lulus seluruh job [CI ini](https://github.com/zalamanga/fti-authentication-assessment/actions/runs/39375132218). Perubahan dokumentasi sesudahnya tidak mengubah source aplikasi.

## Pemeriksaan aplikasi

| Pemeriksaan | Hasil | Bukti |
| --- | --- | --- |
| Format, static analysis, unit/integration PostgreSQL | PASS | Job backend: `gofmt`, `go vet ./...`, `go test -race ./...` dengan Go 1.27.3. |
| Batas dua sesi | PASS | Integration test membuat 12 sesi secara paralel, memeriksa jumlah akhir dua, dan memastikan token tertua ditolak. |
| Force logout dan otorisasi | PASS | Token kedua perangkat ditolak setelah force logout; user mendapat 403 pada endpoint admin; sesi admin tetap berlaku. |
| Registrasi, expiry, ownership | PASS | Tes mencakup rollback registrasi jika pembuatan sesi gagal, uniqueness tanpa membedakan kapital, email/username login, expiry, dan larangan mencabut sesi akun lain. |
| Kerentanan Go | PASS | `govulncheck` v1.9.0 selesai dengan exit 0 pada source deployment Linux. |
| Frontend | PASS | `npm ci` dan `npm run build` lulus di CI; audit npm lokal pada lockfile yang sama melaporkan 0 kerentanan. |
| Docker Compose | PASS | Ketiga layanan berhasil dibangun dan dijalankan; frontend port 3000, backend port 8080, dan proxy `/api/health` merespons. Migrasi serta admin disiapkan oleh startup aplikasi. |
| Browser | PASS | Keempat tes Playwright di bawah lulus pada stack Docker menggunakan Chromium. |
| Publikasi | PASS | GitHub API anonim menyatakan `private=false`, `visibility=public`; `git ls-remote` tanpa credential dapat membaca branch `main`. |

CI memakai `docker compose --env-file .env.example up --detach --build --wait`. Nilai lingkungannya sama dengan menyalin `.env.example` menjadi `.env` seperti petunjuk pengumpulan. Tidak ada perintah migrasi atau seed tambahan. Host Windows dipakai untuk build frontend, audit npm, pemeriksaan sintaks Nginx, serta pemeriksaan screenshot; Go, race detector, dan Docker diverifikasi pada runner Linux.

## Catatan pengoperasian kontrol

Skenario berada pada [frontend/tests/auth.spec.ts](../frontend/tests/auth.spec.ts).

| Skenario | Hasil pengoperasian |
| --- | --- |
| Halaman anonim dan keyboard | PASS: Masuk/Daftar berpindah halaman; Tab mencapai input dan tombol; Enter mengubah visibilitas kata sandi; tema terang/gelap berubah; login diperiksa pada 320/360/768/1280 piksel dan ukuran teks 200% pada desktop. |
| Authentication dan sesi | PASS: konfirmasi kata sandi salah menampilkan error; daftar membuat akun dan membuka profil; login menerima email/username; password salah ditolak; muat ulang sesi bekerja; user tidak mendapat menu admin dan halaman admin menampilkan akses terbatas. |
| Pencabutan sesi | PASS: Escape dan Batal menutup dialog; Akhiri sesi mencabut perangkat lain maupun perangkat saat ini; login ketiga mencabut token pertama; force logout mencabut kedua token pengguna. |
| Admin dan tema | PASS: Cari memfilter hasil; pencarian kosong menampilkan empty state; Hapus filter memulihkan daftar; tema dan overflow diperiksa pada 360/768/1280 piksel; profil mobile diperiksa pada kedua tema. |
| Pagination dan pemulihan error | PASS: Berikutnya membuka halaman 2, Sebelumnya kembali ke halaman 1, tombol batas dinonaktifkan; Muat ulang menampilkan error data lalu pulih setelah server kembali tersedia. Kasus pagination memakai 21 fixture yang secara eksplisit diberi nama `fixture_user_*`, bukan data seed aplikasi. |
| Koneksi awal gagal | PASS: kegagalan jaringan menampilkan pesan dan tombol Coba hubungkan kembali; retry memulihkan halaman login. |
| JavaScript | PASS: skenario authentication mencatat event `pageerror` dan tidak menemukan exception aplikasi. Respons 401 yang diharapkan dan kegagalan jaringan yang disengaja merupakan bagian tes. |

Screenshot masuk tersedia di [desktop](screenshots/login-desktop.png) dan [mobile gelap](screenshots/login-mobile-dark.png). Screenshot profil dan admin dari eksekusi browser juga telah diperiksa secara visual: data membungkus, tabel menjadi susunan vertikal pada mobile, dan kontrol tetap dapat dijangkau.

## Delivery gate antislop

Arah desain dari pengguna dan alasan keputusan tercatat di [DESIGN.md](../DESIGN.md). ENERGY 1 / RHYTHM 2 / MOTION 1.

### Hard gate

- R-02 PASS: teks antarmuka tidak memakai em dash.
- R-03 PASS: pemeriksaan overflow browser dan screenshot pada ukuran yang disebutkan di atas tidak menemukan luapan atau konten terpotong.
- R-17 PASS: jumlah pengguna/sesi berasal dari API; angka batas dua sesi berasal dari spesifikasi asesmen.
- R-18 PASS: tidak ada testimonial atau identitas orang rekaan.
- R-23 PASS: identitas berupa nama produk dalam teks; tidak ada logo/foto rekaan; navigasi mengikuti halaman yang diminta asesmen.
- R-24 PASS: Masuk, Daftar, Profil, dan Pengguna memiliki tujuan yang diimplementasikan.
- R-25 PASS: perhitungan WCAG atas 43 pasangan warna teks memiliki rasio terendah 5,05:1; tiga pasangan batas input terendah 3,47:1. Kontrol nonaktif dikecualikan.
- R-26 PASS: formulir, tautan, toggle, pencarian, pagination, retry, logout, dan dialog memiliki perilaku yang diuji.
- R-27 PASS: halaman data memiliki loading, empty, dan error state; pencarian tanpa hasil serta pemulihan kegagalan jaringan/data diuji.
- R-28 PASS: tidak ada FAQ pengisi.
- R-32 PASS: input/tombol dapat dioperasikan dengan keyboard, focus outline tersedia, dan dialog menerima Escape serta mengembalikan fokus.
- R-33 PASS: fitur ditulis pada source TSX/CSS; tidak ada script eksternal yang menambal fitur UI.
- R-34 PASS: kedua tema diuji dan pasangan warna keduanya dihitung.
- R-35 PASS: build dan empat tes browser lulus; pengoperasian kontrol dicatat pada tabel di atas.
- R-36 PASS: UI dan README tidak mengklaim sertifikasi atau jaminan keamanan tanpa bukti.
- R-37 PASS: pengguna memilih arah profesional hangat sebelum pembuatan UI; arah dan dials tercatat.
- R-38 PASS: data akun berasal dari backend; fixture berada hanya dalam tes dan diberi nama eksplisit; screenshot dokumentasi masuk tidak berisi akun rekaan.

### Purpose gate

- R-01 PASS: krem/hijau tua/terakota mengikuti pilihan pengguna; tidak ada gradient/glow dekoratif.
- R-04 PASS: label teks dipakai tanpa pustaka ikon dekoratif.
- R-06 PASS: Georgia untuk karakter editorial dan sistem sans-serif untuk isian/data; alasannya tercatat di DESIGN.md.
- R-07 PASS: latar polos membantu pembacaan formulir; tidak ada pola grid dekoratif.
- R-08 PASS: tombol memakai label tindakan, tanpa panah dekoratif.
- R-09 PASS: label peran dan perangkat saat ini membawa informasi aktual, tanpa kapsul promosi.
- R-10 PASS: tidak ada glassmorphism.
- R-12 PASS: bayangan hanya pada dialog yang berada di atas halaman.
- R-13 PASS: tidak ada glow.
- R-14 PASS: profil memakai lembar data dan sesi memakai daftar; tidak ada grid kartu fitur pengisi.
- R-19 PASS: motion dibatasi pada hover/fokus dan perubahan warna; reduced motion dihormati.
- R-22 PASS: tidak ada ilustrasi generik atau visual yang tidak berhubungan dengan tugas.

### Liveliness

- Dials PASS: ENERGY 1 / RHYTHM 2 / MOTION 1 dinyatakan dan sesuai tampilan yang diperiksa.
- Focal point PASS: formulir menjadi fokus halaman akses, identitas akun menjadi fokus profil, dan daftar pengguna menjadi fokus admin.
- Whitespace PASS: jarak memisahkan isian, informasi akun, serta pengelolaan sesi.
- Accent PASS: terakota menandai tindakan utama dan tindakan pencabutan sesi.
- Motif PASS: judul serif dan garis horizontal berulang pada formulir, profil, serta admin.
- Design read PASS: jenis portal, audiens internal, arah hangat, dan ketiga dials dinyatakan sebelum implementasi.

### Craftsmanship dan quality locks

- C-1 PASS: alasan warna, layout, tipografi, spacing, permukaan, dan motion tertulis di DESIGN.md.
- C-2 PASS: seluruh kontrol yang ditampilkan memiliki tindakan; keadaan disabled dipakai pada batas pagination dan proses yang sedang berjalan.
- C-3 PASS: konten mengikuti tugas akun/sesi/admin; tidak ada bagian pemasaran pengisi.
- C-4 PASS: error, retry, keyboard, mobile, dan kedua tema diperiksa melalui tes serta screenshot.
- C-5 PASS: angka dan klaim mengacu pada kebutuhan asesmen atau hasil eksekusi yang ditautkan.
- R-05 PASS: formulir terpisah dari konteks sesi; profil menggunakan lembar dan daftar; admin menggunakan tabel yang beralih susunan pada mobile.
- R-11 PASS: radius 4/6/8 piksel membedakan kontrol, lembar profil, dan dialog; elemen tidak seluruhnya berbentuk pil.
- R-15 PASS: CTA menjelaskan aksi, seperti Masuk ke akun, Buat akun, Akhiri sesi, dan Cabut seluruh sesi.
- R-16 PASS: copy menjelaskan fungsi tanpa jargon pemasaran AI.
- R-20 PASS: arah hangat, tipografi editorial, dan konteks FTI konsisten pada seluruh halaman.
- R-21 PASS: pengguna dapat memilih tema terang atau gelap dan preferensinya disimpan.
- R-29 PASS: palet dibatasi pada hijau tua, krem, terakota, dan warna netral.
- R-30 PASS: tampilan mengikuti arah yang dipilih pengguna, tanpa meniru produk tertentu.
- R-31 PASS: alasan seluruh keputusan visual utama tercatat pada DESIGN.md.
