import type { Resource } from "../lib/useResource";

// 読み込み中・空・取得失敗・再認証要求を区別して表示する。

export function LoadingMessage({ label }: { label: string }) {
  return (
    <p className="state-message" role="status">
      {label}を読み込み中…
    </p>
  );
}

export function EmptyMessage({ label }: { label: string }) {
  return <p className="state-message">{label}</p>;
}

export function RefreshingBadge() {
  return (
    <span className="refreshing-badge" role="status">
      更新中…
    </span>
  );
}

export function ErrorBanner({ resource }: { resource: Resource<unknown> }) {
  if (resource.authRequired) {
    return (
      <div className="banner banner-error" role="alert">
        <p>セッションが失効しました。再読み込みして再認証してください。</p>
        <button type="button" onClick={() => window.location.reload()}>
          ページを再読み込み
        </button>
      </div>
    );
  }
  return (
    <div className="banner banner-error" role="alert">
      <p>取得に失敗しました: {resource.error}</p>
      <button type="button" onClick={resource.reload}>
        再試行
      </button>
    </div>
  );
}
