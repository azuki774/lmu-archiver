import type { RatingSnapshot } from "../api/types";
import { formatDateTime } from "./format";

// Brush の選択インデックスを観測日時へ変換し、API の期間境界
// （from は含む、to は含まない）に組み立てる。
// タイムスタンプはマイクロ秒精度のため BigInt で演算する（Date はミリ秒精度）。

export interface BrushRange {
  startIndex: number;
  endIndex: number;
}

export interface Period {
  from: string;
  to: string;
}

function toMicros(iso: string): bigint {
  const match = /\.(\d+)?(?:Z|[+-]\d{2}:\d{2})$/.exec(iso);
  const fractional = match?.[1] ?? "";
  // マイクロ秒未満は切り捨て、ミリ秒未満の端数を BigInt で補う。
  const subMilli = BigInt(fractional.padEnd(6, "0").slice(0, 6)) % 1000n;
  return BigInt(Date.parse(iso)) * 1000n + subMilli;
}

function fromMicros(micros: bigint): string {
  const date = new Date(Number(micros / 1000n));
  const seconds = date.toISOString().slice(0, 19);
  const fraction = String(micros % 1_000_000n).padStart(6, "0");
  return `${seconds}.${fraction}Z`;
}

// 全範囲の選択は期間フィルターなし（全レース）を意味する。
// 部分選択は from=観測[startIndex]（含む）、to=観測[endIndex]+1µs（含まない）。
export function brushRangeToPeriod(series: RatingSnapshot[], range: BrushRange | null): Period | null {
  if (series.length === 0 || range === null) return null;
  const startIndex = Math.max(0, Math.min(range.startIndex, series.length - 1));
  const endIndex = Math.max(startIndex, Math.min(range.endIndex, series.length - 1));
  if (startIndex === 0 && endIndex === series.length - 1) return null;
  const first = series.at(startIndex);
  const last = series.at(endIndex);
  if (first === undefined || last === undefined) return null;
  return {
    from: first.observedAt,
    to: fromMicros(toMicros(last.observedAt) + 1n),
  };
}

export function describePeriod(series: RatingSnapshot[], range: BrushRange | null): string {
  if (series.length === 0) return "観測なし";
  if (range === null) return "全期間";
  const startIndex = Math.max(0, Math.min(range.startIndex, series.length - 1));
  const endIndex = Math.max(startIndex, Math.min(range.endIndex, series.length - 1));
  if (startIndex === 0 && endIndex === series.length - 1) return "全期間";
  const first = series.at(startIndex);
  const last = series.at(endIndex);
  if (first === undefined || last === undefined) return "全期間";
  return `${formatDateTime(first.observedAt)} 〜 ${formatDateTime(last.observedAt)}`;
}
