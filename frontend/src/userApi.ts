import type { AuthSession, AuthUser } from "./auth";

const API_BASE = "";

/**
 * 后端统一响应结构（来自 api/response.Response）。
 * 成功时 code = 0；失败时 code 是业务错误码、msg 是中文 message。
 */
interface BackendResponse<T> {
  code: number;
  msg: string;
  data: T;
}

export interface UserAPIError extends Error {
  code: number;
  status: number;
}

function makeError(message: string, code: number, status: number): UserAPIError {
  const err = new Error(message) as UserAPIError;
  err.code = code;
  err.status = status;
  return err;
}

async function postJSON<T>(path: string, body: unknown, token = ""): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (token) headers["Authorization"] = `Bearer ${token}`;

  const res = await fetch(`${API_BASE}${path}`, {
    method: "POST",
    headers,
    body: JSON.stringify(body),
  });

  let payload: BackendResponse<T> | null = null;
  try {
    payload = (await res.json()) as BackendResponse<T>;
  } catch {
    // 解析失败统一兜成 5xx 错误。
    throw makeError(`接口返回非 JSON：${res.status}`, -1, res.status);
  }

  if (!res.ok || payload.code !== 0) {
    throw makeError(payload?.msg || `请求失败：${res.status}`, payload?.code ?? -1, res.status);
  }

  return payload.data;
}

interface RegisterPayload {
  user: AuthUser;
}

interface LoginPayload {
  token: string;
  user: AuthUser;
}

interface UpdateProfilePayload {
  user: AuthUser;
}

export interface RegisterRequest {
  email: string;
  password: string;
  username: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface UpdateProfileRequest {
  username?: string;
  password?: string;
}

/** 注册成功后返回 user，调用方仍需走 login 拿 token。 */
export async function register(req: RegisterRequest): Promise<AuthUser> {
  const data = await postJSON<RegisterPayload>("/api/user/register", req);
  return data.user;
}

/** 登录成功后返回 token + user，可直接用来填充 AuthSession。 */
export async function login(req: LoginRequest): Promise<AuthSession> {
  const data = await postJSON<LoginPayload>("/api/user/login", req);
  return { token: data.token, user: data.user };
}

/** 更新当前登录用户的资料；token 必填。 */
export async function updateProfile(token: string, req: UpdateProfileRequest): Promise<AuthUser> {
  const data = await postJSON<UpdateProfilePayload>("/api/user/profile", req, token);
  return data.user;
}
