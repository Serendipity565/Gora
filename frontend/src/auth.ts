/**
 * 当前登录态的 token / user 在浏览器侧的本地存储与广播。
 *
 * - token / user 都放 localStorage，刷新页面不掉登录；
 * - 提供 getToken / setSession / clearSession 三个原子操作；
 * - 通过自定义事件 `gora:auth` 让 main.ts 在登录 / 退出时切换视图。
 */

const TOKEN_KEY = "gora.token";
const USER_KEY = "gora.user";
const AUTH_EVENT = "gora:auth";

export interface AuthUser {
  id: number;
  email: string;
  username: string;
  status: number;
}

export interface AuthSession {
  token: string;
  user: AuthUser;
}

function safeGet(key: string): string {
  try {
    return window.localStorage.getItem(key) ?? "";
  } catch {
    return "";
  }
}

function safeSet(key: string, value: string): void {
  try {
    if (value) window.localStorage.setItem(key, value);
    else window.localStorage.removeItem(key);
  } catch {
    // noop
  }
}

export function getToken(): string {
  return safeGet(TOKEN_KEY);
}

export function getUser(): AuthUser | null {
  const raw = safeGet(USER_KEY);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as AuthUser;
  } catch {
    return null;
  }
}

export function isLoggedIn(): boolean {
  return getToken().length > 0;
}

export function setSession(session: AuthSession): void {
  safeSet(TOKEN_KEY, session.token);
  safeSet(USER_KEY, JSON.stringify(session.user));
  dispatchAuthChange("login");
}

export function clearSession(): void {
  safeSet(TOKEN_KEY, "");
  safeSet(USER_KEY, "");
  dispatchAuthChange("logout");
}

type AuthChangeReason = "login" | "logout";

export interface AuthChangeDetail {
  reason: AuthChangeReason;
}

function dispatchAuthChange(reason: AuthChangeReason): void {
  window.dispatchEvent(new CustomEvent<AuthChangeDetail>(AUTH_EVENT, { detail: { reason } }));
}

export function onAuthChange(handler: (detail: AuthChangeDetail) => void): () => void {
  const listener = (e: Event) => handler((e as CustomEvent<AuthChangeDetail>).detail);
  window.addEventListener(AUTH_EVENT, listener);
  return () => window.removeEventListener(AUTH_EVENT, listener);
}
