# FTI Access

Aplikasi authentication untuk asesmen Fullstack Developer Divisi Future Technology and Innovation, PT Kuliner Kreasindo Inovasi. Backend memakai Go dan PostgreSQL; frontend memakai React, TypeScript, dan Vite, lalu disajikan melalui Nginx. Seluruh layanan dijalankan dengan Docker Compose.

![Halaman masuk FTI Access](docs/screenshots/login-desktop.png)

## Fitur

- Pendaftaran dengan email, username, dan kata sandi. Akun baru selalu berperan `user` dan langsung masuk.
- Login menggunakan email atau username tanpa membedakan huruf besar/kecil; logout mengakhiri sesi saat ini.
- Profil pribadi yang hanya dapat diakses setelah login.
- Maksimal **dua sesi aktif per akun**. Login ketiga mencabut sesi paling lama, termasuk ketika beberapa login berlangsung bersamaan.
- Admin dapat mencari dan melihat daftar pengguna dengan pagination, jumlah sesi aktif, serta memaksa pengguna lain keluar dari seluruh sesi.
- Tambahan: daftar sesi pribadi, penanda sesi saat ini, dan pencabutan satu sesi/perangkat dari halaman profil.
- Tema terang/gelap, tampilan responsif, serta dialog konfirmasi pencabutan sesi yang dapat dioperasikan dengan keyboard.
- Validasi input, password hashing, pembatasan percobaan login/daftar, pemeriksaan origin, serta tampilan loading, error, dan sesi berakhir.

## Menjalankan aplikasi

Prasyarat: Git dan Docker dengan Compose plugin. Port `3000` serta `8080` harus tersedia. Tidak diperlukan Go, Node.js, atau PostgreSQL pada host.

```bash
git clone https://github.com/zalamanga/fti-authentication-assessment.git
cd fti-authentication-assessment
cp .env.example .env
docker compose up -d --build
```

Di PowerShell, `cp` juga tersedia sebagai alias `Copy-Item`. Setelah layanan siap, buka frontend di **http://localhost:3000**. Backend tersedia di **http://localhost:8080**, dengan pemeriksaan kesiapan di **http://localhost:8080/api/health**.

PostgreSQL harus sehat sebelum backend dijalankan. Backend menerapkan [migrasi SQL](backend/migrations/001_init.sql) dan membuat admin pertama secara otomatis sebelum menerima permintaan. Frontend menunggu backend sehat. Pembuatan database, migrasi, dan admin tidak memerlukan perintah tambahan.

Kredensial admin awal sesuai `.env.example`:

| Kolom | Nilai |
| --- | --- |
| Email | `admin@fti.local` |
| Username | `admin` |
| Kata sandi | `ChangeMeNow!2026` |

Masuk menggunakan email atau username tersebut, lalu buka menu **Pengguna**. Kredensial contoh ini ditujukan untuk penilaian lokal. Untuk instalasi lain, ubah `ADMIN_*` sebelum startup pertama. Jika sudah ada admin, restart tidak mengganti kata sandinya dan tidak membuat admin tambahan.

Data bertahan di named volume `postgres_data` saat `docker compose down` lalu dijalankan kembali. Gunakan `docker compose logs -f backend` untuk melihat kegagalan startup. Mengubah kredensial PostgreSQL dalam `.env` tidak mengubah kredensial database yang sudah tersimpan pada volume lama.

## Konfigurasi

| Variabel | Nilai contoh | Keterangan |
| --- | --- | --- |
| `POSTGRES_DB` | `fti_auth` | Nama database |
| `POSTGRES_USER` | `fti` | Pengguna database |
| `POSTGRES_PASSWORD` | `fti_dev_password_2026` | Kata sandi database |
| `ADMIN_EMAIL` | `admin@fti.local` | Email admin bootstrap |
| `ADMIN_USERNAME` | `admin` | Username admin bootstrap |
| `ADMIN_PASSWORD` | `ChangeMeNow!2026` | Kata sandi admin bootstrap |
| `APP_ORIGIN` | `http://localhost:3000` | Origin frontend yang diizinkan, tanpa trailing slash |
| `COOKIE_SECURE` | `false` | `false` untuk HTTP lokal; gunakan `true` dengan HTTPS |
| `SESSION_TTL_HOURS` | `168` | Masa berlaku absolut sesi, dalam jam; rentang 1 sampai 8760 |

Compose membentuk `DATABASE_URL` dari `POSTGRES_*`. Gunakan huruf, angka, `_`, atau `-` pada kredensial database agar aman digunakan langsung dalam URL. Database hanya diakses melalui jaringan internal Compose. Berkas `.env` diabaikan Git; `.env.example` berisi contoh konfigurasi yang dapat dikumpulkan.

## Endpoint

Semua endpoint berada di backend port `8080`. Frontend menggunakan URL relatif `/api` yang diteruskan Nginx ke backend. Permintaan dengan body memakai `Content-Type: application/json`. Authentication menggunakan cookie `fti_session`; browser mengirim cookie otomatis dan klien API perlu menyimpan cookie jar. Token tidak dikembalikan dalam JSON.

| Metode | Endpoint | Akses | Body / hasil utama |
| --- | --- | --- | --- |
| `GET` | `/api/health` | Publik | `{ "status": "ok" }` jika database siap |
| `POST` | `/api/auth/register` | Publik | Field `email`, `username`, `password`; membuat user dan sesi |
| `POST` | `/api/auth/login` | Publik | Field `identifier`, `password`; identifier berupa email/username |
| `POST` | `/api/auth/logout` | Publik, idempoten | Mencabut sesi cookie saat ini dan menghapus cookie |
| `GET` | `/api/auth/me` | Login | `{ "user": ... }` milik pemilik sesi |
| `GET` | `/api/sessions` | Login | `{ "sessions": [...] }` dengan perangkat, waktu, serta penanda `current` |
| `DELETE` | `/api/sessions/{id}` | Pemilik sesi | Mencabut satu sesi milik sendiri |
| `GET` | `/api/admin/users?page=1&q=...` | Admin | `users`, `total`, `page`, dan `page_size` |
| `POST` | `/api/admin/users/{id}/logout` | Admin | Mencabut seluruh sesi pengguna lain |

Login dan pendaftaran mengembalikan objek `user` dengan field `id`, `email`, `username`, `role`, dan `created_at`. Daftar admin juga memuat `active_sessions`. Error menggunakan bentuk `{ "error": "pesan", "code": "KODE_ERROR" }`. Respons utama: `400` untuk input salah, `401` untuk sesi tidak valid, `403` untuk akses yang ditolak, `409` untuk email/username yang sudah digunakan, dan `429` ketika batas percobaan terlampaui. Username harus 3 sampai 32 karakter berupa huruf, angka, atau underscore; kata sandi 10 sampai 72 byte.

## Keputusan teknis

- **Sesi di database, tanpa JWT.** Token acak 32 byte berada dalam cookie `HttpOnly`, `SameSite=Lax`, dengan opsi `Secure`. Database hanya menyimpan SHA-256 token. [Middleware `requireAuth`](backend/middleware.go) memeriksa keberadaan dan kedaluwarsa sesi di PostgreSQL pada setiap permintaan terlindungi, sehingga pencabutan berlaku pada permintaan berikutnya tanpa menunggu token habis masa berlaku.
- **Batas dua sesi bersifat transaksional.** [`Store.issueSessionTx`](backend/store.go) mengunci baris pengguna dengan `SELECT ... FOR UPDATE`, membuang sesi kedaluwarsa dan seluruh sesi lama kecuali satu yang terbaru, lalu membuat sesi baru dalam transaksi yang sama. Login memanggilnya melalui `Store.issueSession`; pendaftaran memakai `Store.registerWithSession` agar pembuatan akun dan sesi berhasil atau batal bersama. `Store.revokeAllSessions` menggunakan kunci baris pengguna yang sama agar login dan force logout memiliki urutan yang konsisten. Dua sesi berarti dua token, bukan dua tab; tab dalam profil browser yang sama berbagi cookie.
- **Otorisasi pada server.** Peran tidak dipercaya dari body atau frontend. `requireAuth(true, ...)` mengambil peran dari database. Pengguna biasa tetap mendapat `403` ketika memanggil endpoint admin langsung.
- **Migrasi dan bootstrap otomatis.** [`migrate` dan `bootstrapAdmin`](backend/store.go) memakai transaction advisory lock agar startup bersamaan tidak menjalankan bootstrap dua kali. Versi migrasi dicatat dalam `schema_migrations`; SQL disertakan dalam binary melalui `go:embed`.
- **Validasi dan pertahanan HTTP.** SQL memakai parameter; unique index PostgreSQL mengamankan email/username secara case-insensitive. [`security`](backend/middleware.go) membatasi origin dan permintaan mutasi lintas situs. Body JSON dibatasi 8 KiB dan field tidak dikenal ditolak. Login serta pendaftaran berbagi batas 20 percobaan per menit per alamat koneksi, dengan respons login generik untuk kredensial salah.
- **Pemisahan layanan.** Docker melakukan build frontend/backend dalam beberapa stage, PostgreSQL memiliki healthcheck serta volume persisten, dan frontend memakai Nginx untuk SPA fallback serta proxy API.

## Pengujian

Workflow [CI](.github/workflows/ci.yml) menjalankan pemeriksaan format Go, `go vet`, unit test dan integration test PostgreSQL dengan race detector, pemindaian kerentanan Go, build frontend, serta validasi dan startup seluruh layanan Compose. Smoke check memeriksa frontend, backend, dan proxy API pada port yang dipublikasikan; Playwright menguji alur authentication langsung pada stack Docker. Workflow berjalan otomatis setelah repositori di-push ke GitHub.

[Hasil CI](https://github.com/zalamanga/fti-authentication-assessment/actions/runs/39375132218) untuk source akhir lulus seluruh job. [Laporan verifikasi](docs/verification.md) memuat bukti uji sesi, pengoperasian kontrol, responsivitas, serta pemeriksaan antarmuka.

Untuk menjalankan pemeriksaan secara lokal dengan Go 1.27 dan Node.js 22.12 atau lebih baru:

```bash
cd backend
go vet ./...
go test ./...
cd ../frontend
npm ci
npm run build
```

Integration test memerlukan `TEST_DATABASE_URL` dan pengguna database dengan izin `CREATE SCHEMA`. Test membuat skema sementara yang terisolasi, kemudian menghapus skema tersebut setelah selesai. Database khusus pengujian tetap disarankan. Contoh pada Bash:

```bash
cd backend
TEST_DATABASE_URL='postgres://fti:password@localhost:5432/fti_auth_test?sslmode=disable' go test -race ./...
```

Pada PowerShell, isi `$env:TEST_DATABASE_URL` sebelum `go test ./...`. Tanpa variabel ini, integration test dilewati dan unit test tetap dijalankan. Race detector membutuhkan toolchain C yang didukung Go.

Frontend juga memiliki pengujian browser Playwright. Backend Go harus sudah berjalan di `http://localhost:8080` dengan migrasi dan admin bootstrap tersedia. Playwright menjalankan Vite di port `3000` secara otomatis, atau memakai frontend yang sudah aktif pada port tersebut saat dijalankan di luar CI.

```bash
cd frontend
npm ci
npx playwright install chromium
npm run test:e2e
```

Test browser membuat akun pengujian dan memakai kredensial admin contoh. Jika kredensial admin berbeda, isi environment `ADMIN_USERNAME` dan `ADMIN_PASSWORD` sebelum menjalankan test. Skenario mencakup pendaftaran, login, pembatasan sesi, larangan akses admin bagi user, force logout, pencabutan sesi, tema, dan pemeriksaan overflow pada beberapa lebar layar.

Skenario penerimaan yang dapat dicoba dari UI:

1. Daftarkan pengguna biasa, periksa profil, logout, lalu login bergantian memakai email dan username.
2. Login akun yang sama pada tiga profil browser/perangkat terpisah. Akses ulang profil pada sesi pertama: permintaan ditolak dan pengguna diminta masuk kembali; dua sesi terbaru tetap aktif.
3. Login admin, cari akun tersebut, lalu paksa keluar. Permintaan terlindungi berikutnya dari kedua sesi pengguna harus ditolak.
4. Pada pengguna biasa, akses `/api/admin/users` secara langsung dan pastikan mendapat `403`. Pada profil, cabut satu sesi sendiri dan periksa bahwa sesi tersebut berakhir.
5. Restart layanan dan pastikan akun, data, serta sesi yang belum kedaluwarsa tetap tersedia.

## Catatan implementasi

### 1. Bagaimana kata sandi disimpan, dan mengapa?

Kata sandi disimpan sebagai hash bcrypt dengan cost `12`, bukan plaintext atau enkripsi yang dapat dibalik. Implementasinya berada pada `Store.registerWithSession`, `Store.bootstrapAdmin`, dan `Store.checkPassword` di [backend/store.go](backend/store.go); salt dibuat otomatis oleh pustaka `golang.org/x/crypto/bcrypt`. Bcrypt dipilih karena memiliki salt dan faktor biaya untuk memperlambat tebakan offline, serta tersedia sebagai pustaka Go yang terpelihara. [Validasi](backend/config.go) membatasi kata sandi hingga 72 byte agar tidak melewati batas input bcrypt, dan hash tidak pernah dimasukkan ke respons API.

### 2. Apa definisi sesi, dan di mana batas dua sesi ditegakkan?

Sesi adalah satu baris pada tabel `sessions` yang terhubung dengan pengguna, berisi hash token acak, user agent, waktu dibuat, dan waktu kedaluwarsa; token aslinya dikirim lewat cookie `fti_session`. Sesi aktif berarti baris masih ada dan `expires_at` masih di masa depan, dengan masa berlaku absolut default 168 jam tanpa perpanjangan otomatis. Batas dua sesi ditegakkan oleh `Store.issueSessionTx` di [backend/store.go](backend/store.go), yang digunakan oleh login melalui `Store.issueSession` dan pendaftaran melalui `Store.registerWithSession`, dengan transaksi dan kunci baris pengguna sehingga login ketiga menghapus sesi terlama sekaligus menangani login paralel.

### 3. Apa yang terjadi jika token/sesi dicuri, dan apa yang dapat dilakukan admin?

Pemegang token curian dapat bertindak sebagai pemilik akun selama sesi masih aktif; token tidak diikat ke alamat IP atau user agent, dan token yang dicuri tidak otomatis terdeteksi. Admin dapat memanggil `POST /api/admin/users/{id}/logout` melalui daftar pengguna; handler `forceLogout` pada [backend/handlers.go](backend/handlers.go) memanggil `Store.revokeAllSessions` di [backend/store.go](backend/store.go). Seluruh token akun itu kemudian gagal pada pemeriksaan database berikutnya, termasuk token curian; permintaan yang sudah lolos pemeriksaan sebelum pencabutan mungkin telah berjalan. Jika kata sandi juga bocor, force logout saja tidak mencegah login ulang dan kredensial perlu diganti melalui prosedur operasional.

### 4. Sebutkan dua kekurangan atau celah yang masih ada.

Pertama, belum ada verifikasi kepemilikan email dan pemulihan/penggantian kata sandi, sehingga pendaftaran pada `register` di [backend/handlers.go](backend/handlers.go) hanya memvalidasi format email dan penanganan kredensial bocor masih membutuhkan intervensi operasional. Kedua, rate limiter pada [backend/middleware.go](backend/middleware.go) tersimpan dalam memori per proses dan memakai alamat koneksi langsung; batasnya hilang ketika proses restart, tidak terbagi antar replika, dan pengguna di belakang proxy Nginx berbagi satu alamat. Penerapan produksi memerlukan penyimpanan limiter bersama serta konfigurasi trusted proxy yang membatasi header alamat klien.

### 5. Bagian mana yang diubah untuk fitur keluar dari seluruh perangkat?

Saya akan menambahkan route login-required `POST /api/auth/logout-all` pada `App.routes` dan handler di [backend/handlers.go](backend/handlers.go), lalu memanggil `Store.revokeAllSessions(ctx, auth.User.ID)` yang sudah ada di [backend/store.go](backend/store.go), serta `clearCookie` agar browser saat ini ikut keluar. Identitas target wajib berasal dari sesi pemanggil, bukan body permintaan. Frontend perlu tombol pada halaman profil dan penanganan sukses yang menghapus state pengguna; integration test ditambahkan untuk memastikan kedua sesi akun tersebut ditolak sementara sesi akun lain tetap berlaku. Skema database tidak perlu diubah.

## Penggunaan AI

OpenAI Codex digunakan untuk membantu merancang struktur aplikasi, menulis backend Go, frontend React, migrasi SQL, konfigurasi Docker/CI, pengujian, dan README, serta meninjau perilaku sesi dan otorisasi. Hasil bantuan AI tetap perlu dipahami dan dipertanggungjawabkan oleh pengumpul saat interview, termasuk transaksi pembatasan sesi dan mekanisme pencabutannya.
