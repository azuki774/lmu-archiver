// 表示用の整形。API の値はそのまま扱い、欠損の見た目だけをここで決める。

export function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleString("ja-JP", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

export function formatChartTime(ms: number): string {
  return new Date(ms).toLocaleString("ja-JP", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

export function formatLapTime(us: number | null): string {
  if (us === null) return "—";
  const milliseconds = us / 1000;
  const minutes = Math.floor(milliseconds / 60000);
  const seconds = (milliseconds % 60000) / 1000;
  return `${minutes}:${seconds.toFixed(3).padStart(6, "0")}`;
}

export function formatDelta(value: number): string {
  const sign = value > 0 ? "+" : "";
  return `${sign}${value.toFixed(2)}`;
}

export function formatAverage(value: number | null): string {
  return value === null ? "—" : value.toFixed(2);
}

export function formatCount(value: number | null): string {
  return value === null ? "—" : String(value);
}

export function formatPosition(position: number | null): string {
  return position === null ? "—" : `P${position}`;
}
