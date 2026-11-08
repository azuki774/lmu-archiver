// 同一オリジンの /api/v1 配下のみを参照する。接続先のホスト名・ポートは
// コードにもビルド成果物にも含めない（本番は Gateway が /api を振り分ける）。

export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

// Cloudflare Access のセッション切れなど、JSON を取得できない状態。
export class AuthRequiredError extends Error {}

export async function getJson<T>(path: string, signal: AbortSignal): Promise<T> {
  let response: Response;
  try {
    response = await fetch(path, { signal, headers: { Accept: "application/json" } });
  } catch (error) {
    if (error instanceof DOMException && error.name === "AbortError") throw error;
    throw new ApiError(0, "API に接続できませんでした");
  }
  if (response.status === 401 || response.status === 403) {
    throw new AuthRequiredError("このページを利用するには再認証が必要です");
  }
  const contentType = response.headers.get("content-type") ?? "";
  if (!contentType.includes("application/json")) {
    // Access のログインページへリダイレクトされた場合など、JSON 以外が返る。
    if (response.ok) throw new AuthRequiredError("このページを利用するには再認証が必要です");
    throw new ApiError(response.status, `API リクエストが失敗しました (${response.status})`);
  }
  if (!response.ok) {
    throw new ApiError(response.status, await errorMessage(response));
  }
  return (await response.json()) as T;
}

async function errorMessage(response: Response): Promise<string> {
  try {
    const body = (await response.json()) as { error?: unknown };
    if (typeof body.error === "string" && body.error !== "") return body.error;
  } catch {
    // ボディの形式に関わらず既定のメッセージへ落ちる。
  }
  return `API リクエストが失敗しました (${response.status})`;
}
