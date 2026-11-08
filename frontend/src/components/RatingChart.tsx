import { useMemo } from "react";
import { Brush, CartesianGrid, ComposedChart, Line, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import type { RatingSnapshot } from "../api/types";
import { formatChartTime, formatDateTime } from "../lib/format";
import type { BrushRange } from "../lib/period";

interface ChartPoint {
  time: number;
  dr: number;
  sr: number;
}

// DR/SR の時系列グラフ。下部の Brush で期間を選ぶ（ドラッグ中も即座に表示へ反映）。
// 選択範囲の確定と API 取得は親コンポーネントのデバウンスが担う。
export function RatingChart({
  series,
  range,
  onRangeChange,
}: {
  series: RatingSnapshot[];
  range: BrushRange | null;
  onRangeChange: (range: BrushRange) => void;
}) {
  // data の配列 identity が変わると Brush のドラッグ状態が初期化されるため
  // series が変わらない限り同じ配列を渡す。
  const points: ChartPoint[] = useMemo(
    () =>
      series.map(snapshot => ({
        time: Date.parse(snapshot.observedAt),
        dr: snapshot.drElo,
        sr: snapshot.srRating,
      })),
    [series],
  );
  const brushBounds =
    range === null ? {} : { startIndex: range.startIndex, endIndex: range.endIndex };
  return (
    <div className="chart-box" aria-label="レートの時系列グラフ">
      <ResponsiveContainer width="100%" height={340}>
        <ComposedChart data={points} margin={{ top: 12, right: 8, bottom: 0, left: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#d8dce2" />
          <XAxis
            dataKey="time"
            type="number"
            domain={["dataMin", "dataMax"]}
            scale="time"
            tickFormatter={formatChartTime}
            stroke="#6b7280"
          />
          <YAxis yAxisId="dr" stroke="#2563eb" domain={["auto", "auto"]} width={60} />
          <YAxis yAxisId="sr" orientation="right" stroke="#059669" domain={["auto", "auto"]} width={44} />
          <Tooltip
            formatter={(value, name) => [Number(value).toFixed(2), name === "dr" ? "DR" : "SR"]}
            labelFormatter={label => formatDateTime(new Date(Number(label)).toISOString())}
          />
          <Line
            yAxisId="dr"
            dataKey="dr"
            name="DR"
            stroke="#2563eb"
            strokeWidth={2}
            dot={series.length <= 60}
          />
          <Line
            yAxisId="sr"
            dataKey="sr"
            name="SR"
            stroke="#059669"
            strokeWidth={2}
            dot={series.length <= 60}
          />
          <Brush
            dataKey="time"
            height={28}
            travellerWidth={8}
            fill="#eef2f7"
            stroke="#94a3b8"
            onChange={next => onRangeChange({ startIndex: next.startIndex, endIndex: next.endIndex })}
            {...brushBounds}
          />
        </ComposedChart>
      </ResponsiveContainer>
    </div>
  );
}
