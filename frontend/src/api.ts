export type User = {
  id: string;
  email: string;
  username: string;
  role: "user" | "admin";
  created_at: string;
};

export type Session = {
  id: string;
  user_agent: string;
  created_at: string;
  expires_at: string;
  current: boolean;
};

export type AdminUser = User & { active_sessions: number };
export type UserList = {
  users: AdminUser[];
  total: number;
  page: number;
  page_size: number;
};

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

export async function api<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const response = await fetch(`/api${path}`, {
    ...options,
    credentials: "same-origin",
    headers: {
      ...(options.body ? { "Content-Type": "application/json" } : {}),
      ...options.headers,
    },
  });
  const data =
    response.status === 204
      ? undefined
      : await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new ApiError(
      response.status,
      data?.code ?? "request_failed",
      data?.error ?? "Permintaan gagal. Silakan coba kembali.",
    );
  }
  return data as T;
}

export function messageOf(error: unknown): string {
  if (error instanceof ApiError) return error.message;
  return "Tidak dapat terhubung ke server. Periksa koneksi lalu coba kembali.";
}
