import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type AnchorHTMLAttributes,
  type FormEvent,
  type ReactNode,
} from "react";
import {
  api,
  ApiError,
  messageOf,
  type User,
  type Session,
  type AdminUser,
  type UserList,
} from "./api";

type Navigate = (path: string, replace?: boolean) => void;
type Call = <T>(path: string, options?: RequestInit) => Promise<T>;

function usePath() {
  const [path, setPath] = useState(window.location.pathname);
  useEffect(() => {
    const update = () => setPath(window.location.pathname);
    window.addEventListener("popstate", update);
    return () => window.removeEventListener("popstate", update);
  }, []);
  const navigate: Navigate = useCallback((next, replace = false) => {
    window.history[replace ? "replaceState" : "pushState"]({}, "", next);
    setPath(next);
    window.scrollTo(0, 0);
  }, []);
  return [path, navigate] as const;
}

function Link({
  to,
  navigate,
  children,
  ...props
}: {
  to: string;
  navigate: Navigate;
  children: ReactNode;
} & AnchorHTMLAttributes<HTMLAnchorElement>) {
  return (
    <a
      {...props}
      href={to}
      onClick={(event) => {
        if (
          !event.ctrlKey &&
          !event.metaKey &&
          !event.shiftKey &&
          !event.altKey &&
          event.button === 0
        ) {
          event.preventDefault();
          navigate(to);
        }
      }}
    >
      {children}
    </a>
  );
}

function ThemeButton() {
  const [dark, setDark] = useState(() => {
    try {
      return localStorage.getItem("fti_theme") === "dark";
    } catch {
      return false;
    }
  });
  useEffect(() => {
    document.documentElement.dataset.theme = dark ? "dark" : "light";
    try {
      localStorage.setItem("fti_theme", dark ? "dark" : "light");
    } catch {
      /* Preference is optional when storage is unavailable. */
    }
  }, [dark]);
  return (
    <button
      className="theme-button"
      aria-pressed={dark}
      onClick={() => setDark(!dark)}
    >
      {dark ? "Tema terang" : "Tema gelap"}
    </button>
  );
}

export function App() {
  const [path, navigate] = usePath();
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [initialError, setInitialError] = useState("");
  const [notice, setNotice] = useState("");
  const [loggingOut, setLoggingOut] = useState(false);

  const restore = useCallback(async () => {
    setLoading(true);
    setInitialError("");
    try {
      setUser((await api<{ user: User }>("/auth/me")).user);
    } catch (error) {
      if (!(error instanceof ApiError && error.status === 401))
        setInitialError(messageOf(error));
    } finally {
      setLoading(false);
    }
  }, []);
  useEffect(() => {
    void restore();
  }, [restore]);

  const call: Call = useCallback(
    async <T,>(endpoint: string, options?: RequestInit) => {
      try {
        return await api<T>(endpoint, options);
      } catch (error) {
        if (error instanceof ApiError && error.status === 401) {
          setUser(null);
          setNotice(
            "Sesi Anda telah berakhir atau dicabut. Silakan masuk kembali.",
          );
          navigate("/login", true);
        }
        throw error;
      }
    },
    [navigate],
  );

  useEffect(() => {
    if (!user) return;
    const check = () => {
      if (document.visibilityState === "visible")
        void call<{ user: User }>("/auth/me").catch(() => {});
    };
    const interval = window.setInterval(check, 30000);
    window.addEventListener("focus", check);
    return () => {
      window.clearInterval(interval);
      window.removeEventListener("focus", check);
    };
  }, [user, call]);

  useEffect(() => {
    if (loading || initialError) return;
    if (!user && path !== "/login" && path !== "/register")
      navigate("/login", true);
    if (user && ["/", "/login", "/register"].includes(path))
      navigate("/profile", true);
  }, [path, user, loading, initialError, navigate]);

  useEffect(() => {
    const name =
      path === "/register"
        ? "Daftar"
        : path === "/profile"
          ? "Profil"
          : path === "/admin/users"
            ? "Pengguna"
            : "Masuk";
    document.title = `${name} | FTI Access`;
    document.getElementById("main")?.focus();
  }, [path, loading]);

  async function logout() {
    setLoggingOut(true);
    try {
      await call("/auth/logout", { method: "POST" });
      setUser(null);
      setNotice("Anda telah keluar dari perangkat ini.");
      navigate("/login", true);
    } catch (error) {
      if (!(error instanceof ApiError && error.status === 401))
        setNotice(messageOf(error));
    } finally {
      setLoggingOut(false);
    }
  }

  return (
    <>
      <a className="skip-link" href="#main">
        Lewati ke konten
      </a>
      <header className="site-header">
        <Link
          className="wordmark"
          to={user ? "/profile" : "/login"}
          navigate={navigate}
        >
          FTI <span>Access</span>
          <small>Portal tim internal</small>
        </Link>
        <div className="header-actions">
          <ThemeButton />
          {user && (
            <button
              className="button secondary small"
              disabled={loggingOut}
              onClick={() => void logout()}
            >
              {loggingOut ? "Keluar…" : "Keluar"}
            </button>
          )}
        </div>
      </header>
      {loading ? (
        <main id="main" tabIndex={-1} className="container standalone">
          <p role="status">Memeriksa sesi Anda…</p>
        </main>
      ) : initialError ? (
        <main id="main" tabIndex={-1} className="container standalone">
          <h1>Koneksi belum tersedia.</h1>
          <p className="alert error" role="alert">
            {initialError}
          </p>
          <button className="button" onClick={() => void restore()}>
            Coba hubungkan kembali
          </button>
        </main>
      ) : !user ? (
        <AuthPage
          register={path === "/register"}
          navigate={navigate}
          notice={notice}
          onSuccess={(next) => {
            setUser(next);
            setNotice("");
            navigate("/profile", true);
          }}
        />
      ) : (
        <div className="authenticated">
          <nav className="app-nav" aria-label="Navigasi akun">
            <Link
              to="/profile"
              navigate={navigate}
              aria-current={path === "/profile" ? "page" : undefined}
            >
              Profil & sesi
            </Link>
            {user.role === "admin" && (
              <Link
                to="/admin/users"
                navigate={navigate}
                aria-current={path === "/admin/users" ? "page" : undefined}
              >
                Pengguna
              </Link>
            )}
            <span className="signed-in">
              Masuk sebagai <strong>{user.username}</strong>
            </span>
          </nav>
          <main id="main" tabIndex={-1} className="container">
            {notice && (
              <p className="alert" role="status">
                {notice}
              </p>
            )}
            {path === "/profile" ? (
              <Profile
                user={user}
                call={call}
                onCurrentRevoked={() => {
                  setUser(null);
                  setNotice("Sesi perangkat ini telah diakhiri.");
                  navigate("/login", true);
                }}
              />
            ) : path === "/admin/users" ? (
              user.role === "admin" ? (
                <Users user={user} call={call} />
              ) : (
                <section className="standalone">
                  <h1>Akses terbatas.</h1>
                  <p>Daftar pengguna hanya dapat diakses administrator.</p>
                  <Link to="/profile" navigate={navigate}>
                    Kembali ke profil
                  </Link>
                </section>
              )
            ) : (
              <section className="standalone">
                <h1>Halaman tidak ditemukan.</h1>
                <Link to="/profile" navigate={navigate}>
                  Kembali ke profil
                </Link>
              </section>
            )}
          </main>
        </div>
      )}
      <footer className="site-footer">
        <span>FTI · Future Technology and Innovation</span>
        <span>PT Kuliner Kreasindo Inovasi</span>
      </footer>
    </>
  );
}

function AuthPage({
  register,
  navigate,
  notice,
  onSuccess,
}: {
  register: boolean;
  navigate: Navigate;
  notice: string;
  onSuccess: (user: User) => void;
}) {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [show, setShow] = useState(false);
  useEffect(() => {
    setError("");
    setShow(false);
  }, [register]);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    const values = new FormData(event.currentTarget);
    const password = String(values.get("password"));
    if (register && password !== values.get("confirm")) {
      setError("Konfirmasi kata sandi belum sama.");
      return;
    }
    const passwordBytes = new TextEncoder().encode(password).length;
    if (passwordBytes > 72 || (register && passwordBytes < 10)) {
      setError("Kata sandi harus 10–72 byte. Huruf ASCII dihitung 1 byte.");
      return;
    }
    setBusy(true);
    try {
      const body = register
        ? {
            email: String(values.get("email")).trim(),
            username: String(values.get("username")).trim(),
            password,
          }
        : { identifier: String(values.get("identifier")).trim(), password };
      onSuccess(
        (
          await api<{ user: User }>(
            register ? "/auth/register" : "/auth/login",
            { method: "POST", body: JSON.stringify(body) },
          )
        ).user,
      );
    } catch (caught) {
      setError(messageOf(caught));
    } finally {
      setBusy(false);
    }
  }
  return (
    <main id="main" tabIndex={-1} className="auth-layout">
      <aside className="auth-intro">
        <div className="intro-copy">
          <p className="intro-label">Ruang kerja dimulai di sini.</p>
          <h1>
            Satu pintu
            <br />
            untuk tim Anda.
          </h1>
          <p>
            Akses akun, periksa sesi, dan lanjutkan pekerjaan Anda bersama tim
            FTI.
          </p>
        </div>
        <div className="policy-list">
          <div>
            <span>01</span>
            <p>
              <strong>Dua sesi, satu akun.</strong> Login di perangkat ketiga
              akan mengakhiri sesi yang paling lama.
            </p>
          </div>
          <div>
            <span>02</span>
            <p>
              <strong>Akses dalam kendali.</strong> Periksa perangkat Anda.
              Administrator dapat mencabut seluruh sesi akun.
            </p>
          </div>
        </div>
      </aside>
      <section className="auth-panel" aria-labelledby="auth-title">
        <div className="auth-switch" aria-label="Pilihan akses">
          <Link
            to="/login"
            navigate={navigate}
            aria-current={!register ? "page" : undefined}
          >
            Masuk
          </Link>
          <Link
            to="/register"
            navigate={navigate}
            aria-current={register ? "page" : undefined}
          >
            Daftar akun
          </Link>
        </div>
        <div className="form-heading">
          <h2 id="auth-title">
            {register ? "Bergabung dengan tim." : "Selamat datang kembali."}
          </h2>
          <p>
            {register
              ? "Buat akun untuk mengakses portal internal."
              : "Masuk menggunakan email atau username Anda."}
          </p>
        </div>
        {notice && (
          <p role="status" className="alert">
            {notice}
          </p>
        )}
        <form
          key={register ? "register" : "login"}
          onSubmit={(event) => void submit(event)}
          aria-busy={busy}
        >
          {error && (
            <p className="alert error" role="alert">
              {error}
            </p>
          )}
          <fieldset disabled={busy}>
            {register ? (
              <>
                <label htmlFor="email">Email</label>
                <input
                  id="email"
                  name="email"
                  type="email"
                  autoComplete="email"
                  required
                  maxLength={254}
                  placeholder="email@perusahaan.com"
                />
                <label htmlFor="username">Username</label>
                <input
                  id="username"
                  name="username"
                  autoComplete="username"
                  required
                  minLength={3}
                  maxLength={32}
                  pattern="[A-Za-z0-9_]+"
                  aria-describedby="username-hint"
                  placeholder="Username Anda"
                />
                <small id="username-hint" className="field-hint">
                  3–32 karakter: huruf, angka, atau garis bawah.
                </small>
              </>
            ) : (
              <>
                <label htmlFor="identifier">Email atau username</label>
                <input
                  id="identifier"
                  name="identifier"
                  autoComplete="username"
                  required
                  maxLength={254}
                  placeholder="Email atau username Anda"
                />
              </>
            )}
            <label htmlFor="password">Kata sandi</label>
            <div className="password-field">
              <input
                id="password"
                name="password"
                type={show ? "text" : "password"}
                autoComplete={register ? "new-password" : "current-password"}
                required
                maxLength={72}
                aria-describedby={register ? "password-hint" : undefined}
              />
              <button
                type="button"
                className="password-toggle"
                aria-controls="password"
                aria-pressed={show}
                onClick={() => setShow(!show)}
              >
                {show ? "Sembunyikan" : "Tampilkan"}
              </button>
            </div>
            {register && (
              <>
                <small id="password-hint" className="field-hint">
                  10–72 byte. Huruf ASCII dihitung 1 byte.
                </small>
                <label htmlFor="confirm">Konfirmasi kata sandi</label>
                <input
                  id="confirm"
                  name="confirm"
                  type={show ? "text" : "password"}
                  autoComplete="new-password"
                  required
                  maxLength={72}
                />
              </>
            )}
            <button className="button submit" type="submit">
              {busy ? "Memproses…" : register ? "Buat akun" : "Masuk ke akun"}
            </button>
          </fieldset>
        </form>
        <p className="form-bottom">
          {register ? "Sudah memiliki akun? " : "Belum memiliki akun? "}
          <Link to={register ? "/login" : "/register"} navigate={navigate}>
            {register ? "Masuk di sini" : "Daftar akun"}
          </Link>
        </p>
      </section>
    </main>
  );
}

const date = (value: string) =>
  new Intl.DateTimeFormat("id-ID", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
function device(agent: string) {
  const browser = /Edg\//.test(agent)
    ? "Microsoft Edge"
    : /Firefox\//.test(agent)
      ? "Firefox"
      : /Chrome\//.test(agent)
        ? "Chrome"
        : /Safari\//.test(agent)
          ? "Safari"
          : "Browser / aplikasi";
  const os = /Android/.test(agent)
    ? "Android"
    : /iPhone|iPad/.test(agent)
      ? "iOS"
      : /Windows/.test(agent)
        ? "Windows"
        : /Macintosh/.test(agent)
          ? "macOS"
          : /Linux/.test(agent)
            ? "Linux"
            : "perangkat lain";
  return `${browser} · ${os}`;
}

function Profile({
  user,
  call,
  onCurrentRevoked,
}: {
  user: User;
  call: Call;
  onCurrentRevoked: () => void;
}) {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [target, setTarget] = useState<Session | null>(null);
  const [busy, setBusy] = useState(false);
  const refresh = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      setSessions((await call<{ sessions: Session[] }>("/sessions")).sessions);
    } catch (caught) {
      setError(messageOf(caught));
    } finally {
      setLoading(false);
    }
  }, [call]);
  useEffect(() => {
    void refresh();
  }, [refresh]);
  async function revoke() {
    if (!target) return;
    setBusy(true);
    try {
      await call(`/sessions/${target.id}`, { method: "DELETE" });
      if (target.current) {
        onCurrentRevoked();
        return;
      }
      setTarget(null);
      setNotice("Sesi perangkat tersebut telah diakhiri.");
      await refresh();
    } catch (caught) {
      setError(messageOf(caught));
      setTarget(null);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <div className="page-heading">
        <div>
          <p className="section-label">Akun Anda</p>
          <h1>Halo, {user.username}.</h1>
          <p>Informasi akun dan sesi yang sedang aktif.</p>
        </div>
        <span className="role-label">
          {user.role === "admin" ? "Administrator" : "Pengguna"}
        </span>
      </div>
      <div className="profile-layout">
        <section className="account-sheet" aria-labelledby="account-title">
          <h2 id="account-title">Profil</h2>
          <dl>
            <div>
              <dt>Username</dt>
              <dd>{user.username}</dd>
            </div>
            <div>
              <dt>Email</dt>
              <dd>{user.email}</dd>
            </div>
            <div>
              <dt>Peran</dt>
              <dd>{user.role === "admin" ? "Admin" : "User"}</dd>
            </div>
            <div>
              <dt>Bergabung</dt>
              <dd>{date(user.created_at)}</dd>
            </div>
          </dl>
          <p className="sheet-note">
            Profil ini hanya menampilkan informasi akun Anda sendiri.
          </p>
        </section>
        <section className="session-section" aria-labelledby="sessions-title">
          <div className="section-heading">
            <div>
              <h2 id="sessions-title">
                Sesi aktif{" "}
                <span className="count">
                  {!loading && !error ? `${sessions.length}/2` : ""}
                </span>
              </h2>
              <p>Kenali perangkat yang mengakses akun Anda.</p>
            </div>
            <button
              className="text-button"
              disabled={loading}
              onClick={() => void refresh()}
            >
              {loading ? "Memuat…" : "Muat ulang"}
            </button>
          </div>
          <p className="session-policy">
            Maksimal dua sesi aktif. Login berikutnya akan menggantikan sesi
            yang paling lama.
          </p>
          {notice && (
            <p className="alert" role="status">
              {notice}
            </p>
          )}
          {error && (
            <p className="alert error" role="alert">
              {error}
            </p>
          )}
          {loading ? (
            <p className="data-state" role="status">
              Memuat sesi aktif…
            </p>
          ) : !error && sessions.length === 0 ? (
            <p className="data-state">
              Tidak ada sesi aktif. Muat ulang untuk memeriksa status terbaru.
            </p>
          ) : (
            !error && (
              <ul className="sessions">
                {sessions.map((session) => (
                  <li key={session.id}>
                    <div className="session-top">
                      <h3>{device(session.user_agent)}</h3>
                      {session.current && (
                        <span className="current-label">Perangkat ini</span>
                      )}
                    </div>
                    <p>Masuk {date(session.created_at)}</p>
                    <p className="muted">Berakhir {date(session.expires_at)}</p>
                    <button
                      className="text-button danger"
                      onClick={() => setTarget(session)}
                    >
                      Akhiri sesi
                      <span className="sr-only">
                        {" "}
                        {device(session.user_agent)}{" "}
                        {session.current ? "perangkat ini" : "perangkat lain"}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            )
          )}
        </section>
      </div>
      {target && (
        <Confirm
          title="Akhiri sesi ini?"
          busy={busy}
          onClose={() => setTarget(null)}
          onConfirm={() => void revoke()}
          confirmLabel="Akhiri sesi"
        >
          <p>
            {target.current
              ? "Anda akan keluar dari perangkat yang sedang digunakan."
              : `Sesi ${device(target.user_agent)} akan dicabut. Perangkat tersebut harus masuk kembali.`}
          </p>
        </Confirm>
      )}
    </>
  );
}

function Users({ user, call }: { user: User; call: Call }) {
  const [result, setResult] = useState<UserList | null>(null);
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const [version, setVersion] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [target, setTarget] = useState<AdminUser | null>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    let active = true;
    setLoading(true);
    setError("");
    void call<UserList>(
      `/admin/users?page=${page}&q=${encodeURIComponent(search)}`,
    )
      .then((data) => {
        if (active) setResult(data);
      })
      .catch((caught) => {
        if (active) setError(messageOf(caught));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [call, page, search, version]);
  async function forceLogout() {
    if (!target) return;
    setBusy(true);
    try {
      await call(`/admin/users/${target.id}/logout`, { method: "POST" });
      setNotice(`Seluruh sesi ${target.username} telah dicabut.`);
      setTarget(null);
      setVersion((v) => v + 1);
    } catch (caught) {
      setError(messageOf(caught));
      setTarget(null);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <div className="page-heading">
        <div>
          <p className="section-label">Administrasi</p>
          <h1>Pengguna tim.</h1>
          <p>Periksa akun terdaftar dan kelola akses sesi pengguna.</p>
        </div>
        <span className="role-label">Khusus admin</span>
      </div>
      <section className="users-section" aria-labelledby="users-title">
        <div className="user-toolbar">
          <h2 id="users-title">
            Daftar pengguna{" "}
            {result && !loading && !error && (
              <span className="count">({result.total})</span>
            )}
          </h2>
          <button
            className="text-button"
            disabled={loading}
            onClick={() => setVersion((v) => v + 1)}
          >
            {loading ? "Memuat…" : "Muat ulang"}
          </button>
        </div>
        <form
          className="search-form"
          onSubmit={(event) => {
            event.preventDefault();
            setSearch(query.trim());
            setPage(1);
            setVersion((v) => v + 1);
          }}
        >
          <label htmlFor="search">Cari email atau username</label>
          <div>
            <input
              id="search"
              type="search"
              value={query}
              maxLength={254}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Cari pengguna"
            />
            <button
              className="button secondary"
              type="submit"
              disabled={loading}
            >
              Cari
            </button>
            {search && (
              <button
                type="button"
                className="text-button"
                onClick={() => {
                  setQuery("");
                  setSearch("");
                  setPage(1);
                }}
              >
                Hapus filter
              </button>
            )}
          </div>
        </form>
        {notice && (
          <p className="alert" role="status">
            {notice}
          </p>
        )}
        {error && (
          <p className="alert error" role="alert">
            {error}
          </p>
        )}
        {loading ? (
          <p className="data-state" role="status">
            Memuat daftar pengguna…
          </p>
        ) : !error && result && result.users.length === 0 ? (
          <div className="data-state">
            <h3>Tidak ada pengguna yang cocok.</h3>
            <p>Coba email atau username lain, atau hapus filter pencarian.</p>
          </div>
        ) : (
          !error &&
          result && (
            <>
              <table className="users-table" role="table">
                <caption className="sr-only">
                  Akun pengguna, jumlah sesi, dan tindakan pencabutan sesi
                </caption>
                <thead role="rowgroup">
                  <tr role="row">
                    <th scope="col" role="columnheader">
                      Pengguna
                    </th>
                    <th scope="col" role="columnheader">
                      Peran
                    </th>
                    <th scope="col" role="columnheader">
                      Sesi aktif
                    </th>
                    <th scope="col" role="columnheader">
                      Bergabung
                    </th>
                    <th scope="col" role="columnheader">
                      Akses
                    </th>
                  </tr>
                </thead>
                <tbody role="rowgroup">
                  {result.users.map((entry) => (
                    <tr key={entry.id} role="row">
                      <td data-label="Pengguna" role="cell">
                        <strong>
                          {entry.username}
                          {entry.id === user.id && (
                            <span className="you-label"> (Anda)</span>
                          )}
                        </strong>
                        <span className="email-value">{entry.email}</span>
                      </td>
                      <td data-label="Peran" role="cell">
                        {entry.role === "admin" ? "Admin" : "User"}
                      </td>
                      <td data-label="Sesi aktif" role="cell">
                        {entry.active_sessions} / 2
                      </td>
                      <td data-label="Bergabung" role="cell">
                        {date(entry.created_at)}
                      </td>
                      <td data-label="Akses" role="cell">
                        {entry.id === user.id ? (
                          <span className="muted">Kelola di profil</span>
                        ) : (
                          <button
                            className="text-button danger"
                            disabled={entry.active_sessions === 0}
                            onClick={() => setTarget(entry)}
                          >
                            Force logout
                            <span className="sr-only"> {entry.username}</span>
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              <div className="pagination">
                <span>
                  Halaman {result.page} dari{" "}
                  {Math.max(1, Math.ceil(result.total / result.page_size))}
                </span>
                <div>
                  <button
                    className="button secondary small"
                    disabled={page <= 1}
                    onClick={() => setPage((p) => p - 1)}
                  >
                    Sebelumnya
                  </button>
                  <button
                    className="button secondary small"
                    disabled={page * result.page_size >= result.total}
                    onClick={() => setPage((p) => p + 1)}
                  >
                    Berikutnya
                  </button>
                </div>
              </div>
            </>
          )
        )}
        <p className="table-note">
          Force logout mencabut seluruh sesi pengguna. Akun tetap tersedia dan
          dapat masuk kembali.
        </p>
      </section>
      {target && (
        <Confirm
          title={`Cabut akses ${target.username}?`}
          busy={busy}
          onClose={() => setTarget(null)}
          onConfirm={() => void forceLogout()}
          confirmLabel="Cabut seluruh sesi"
        >
          <p>
            Semua sesi aktif milik <strong>{target.username}</strong> akan
            berakhir. Pengguna harus masuk kembali di setiap perangkat.
          </p>
        </Confirm>
      )}
    </>
  );
}

function Confirm({
  title,
  children,
  busy,
  onClose,
  onConfirm,
  confirmLabel,
}: {
  title: string;
  children: ReactNode;
  busy: boolean;
  onClose: () => void;
  onConfirm: () => void;
  confirmLabel: string;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const previouslyFocused = document.activeElement as HTMLElement | null;
    ref.current?.showModal();
    return () => {
      previouslyFocused?.focus();
    };
  }, []);
  return (
    <dialog
      ref={ref}
      className="confirm-dialog"
      aria-labelledby="confirm-title"
      onCancel={(event) => {
        event.preventDefault();
        if (!busy) onClose();
      }}
    >
      <h2 id="confirm-title">{title}</h2>
      {children}
      <div className="dialog-actions">
        <button
          className="button secondary"
          autoFocus
          disabled={busy}
          onClick={onClose}
        >
          Batal
        </button>
        <button
          className="button destructive"
          disabled={busy}
          onClick={onConfirm}
        >
          {busy ? "Mencabut sesi…" : confirmLabel}
        </button>
      </div>
    </dialog>
  );
}
