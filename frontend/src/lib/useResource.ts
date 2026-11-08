import { useEffect, useRef, useState } from "react";
import { AuthRequiredError, getJson } from "../api/client";

// 1 つの取得単位の状態。読み込み中・空・失敗・更新中を区別する。
// path が変われるたびに前のリクエストを abort し、世代番号で
// 遅れて返った古い結果が新しい条件の表示を上書きしないようにする。
export interface Resource<T> {
  status: "loading" | "ready" | "error";
  data: T | null;
  error: string | null;
  authRequired: boolean;
  refreshing: boolean;
  reload: () => void;
}

export function useResource<T>(path: string): Resource<T> {
  const [state, setState] = useState<Omit<Resource<T>, "reload">>({
    status: "loading",
    data: null,
    error: null,
    authRequired: false,
    refreshing: false,
  });
  const [nonce, setNonce] = useState(0);
  const generation = useRef(0);

  useEffect(() => {
    const current = ++generation.current;
    const controller = new AbortController();
    // 前の結果がある間は更新中であることを示しつつ表示を維持する。
    setState(previous =>
      previous.data === null
        ? { status: "loading", data: null, error: null, authRequired: false, refreshing: false }
        : { ...previous, status: "loading", error: null, authRequired: false, refreshing: true },
    );
    getJson<T>(path, controller.signal).then(
      data => {
        if (current !== generation.current) return;
        setState({ status: "ready", data, error: null, authRequired: false, refreshing: false });
      },
      error => {
        if (current !== generation.current) return;
        if (error instanceof DOMException && error.name === "AbortError") return;
        const authRequired = error instanceof AuthRequiredError;
        const message = error instanceof Error && error.message !== "" ? error.message : "取得に失敗しました";
        setState({ status: "error", data: null, error: message, authRequired, refreshing: false });
      },
    );
    return () => controller.abort();
  }, [path, nonce]);

  return { ...state, reload: () => setNonce(value => value + 1) };
}
