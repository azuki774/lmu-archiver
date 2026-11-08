import type { TrackStats } from "../api/types";
import { formatAverage, formatLapTime, formatPosition } from "../lib/format";

// トラック別戦績。API が選択条件に一致する全レースを集計済みの値を返すため、
// 一覧の現在ページは集計対象に関係しない。総合順位とクラス順位は別列。
export function TrackStatsTable({ stats }: { stats: TrackStats[] }) {
  return (
    <table className="data-table stats-table">
      <thead>
        <tr>
          <th rowSpan={2}>トラック</th>
          <th rowSpan={2}>出走</th>
          <th rowSpan={2}>レース</th>
          <th colSpan={4}>総合順位</th>
          <th colSpan={4}>クラス順位</th>
          <th rowSpan={2}>ベストラップ</th>
        </tr>
        <tr>
          <th>勝利</th>
          <th>表彰台</th>
          <th>平均</th>
          <th>最高</th>
          <th>勝利</th>
          <th>表彰台</th>
          <th>平均</th>
          <th>最高</th>
        </tr>
      </thead>
      <tbody>
        {stats.map(row => (
          <tr key={row.track}>
            <td>{row.track}</td>
            <td>{row.starts}</td>
            <td>{row.races}</td>
            <td>{row.overall.wins}</td>
            <td>{row.overall.podiums}</td>
            <td>{formatAverage(row.overall.avgPosition)}</td>
            <td>{formatPosition(row.overall.bestPosition)}</td>
            <td>{row.class.wins}</td>
            <td>{row.class.podiums}</td>
            <td>{formatAverage(row.class.avgPosition)}</td>
            <td>{formatPosition(row.class.bestPosition)}</td>
            <td>{formatLapTime(row.bestLapUs)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
