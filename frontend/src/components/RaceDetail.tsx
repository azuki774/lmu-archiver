import type { Race, SessionResult, SessionType } from "../api/types";
import { formatCount, formatDateTime, formatLapTime, formatPosition } from "../lib/format";

const SESSION_LABELS: Record<SessionType, string> = {
  practice: "練習",
  qualifying: "予選",
  warmup: "ウォームアップ",
  race: "レース",
};

const SESSION_ORDER: Record<SessionType, number> = {
  practice: 1,
  qualifying: 2,
  warmup: 3,
  race: 4,
};

function sessionLabel(row: SessionResult): string {
  const label = SESSION_LABELS[row.sessionType];
  if (row.sessionType === "race" && row.raceNo > 1) return `${label} #${row.raceNo}`;
  return label;
}

// 選択したレースと比較対象のセッション詳細。referenceRating は
// 「レース後の確定レート」ではないため表示しない。
export function RaceDetailCard({
  heading,
  race,
  onClear,
}: {
  heading: string;
  race: Race;
  onClear: () => void;
}) {
  const sessions = [...race.results].sort(
    (a, b) =>
      SESSION_ORDER[a.sessionType] - SESSION_ORDER[b.sessionType] ||
      a.raceNo - b.raceNo ||
      a.sessionNo.localeCompare(b.sessionNo),
  );
  return (
    <article className="detail-card">
      <header className="detail-header">
        <h3>
          {heading}: {race.event.title}
        </h3>
        <button type="button" onClick={onClear}>
          閉じる
        </button>
      </header>
      <dl className="detail-meta">
        <div>
          <dt>日時</dt>
          <dd>{formatDateTime(race.event.eventAt)}</dd>
        </div>
        <div>
          <dt>トラック</dt>
          <dd>{race.event.track}</dd>
        </div>
        <div>
          <dt>クラス</dt>
          <dd>{race.participation.carClass}</dd>
        </div>
        <div>
          <dt>種別</dt>
          <dd>{race.event.eventType}</dd>
        </div>
      </dl>
      <table className="data-table session-table">
        <thead>
          <tr>
            <th>セッション</th>
            <th>順位</th>
            <th>クラス順位</th>
            <th>グリッド</th>
            <th>クラスグリッド</th>
            <th>周回</th>
            <th>ベストラップ</th>
            <th>フィニッシュ</th>
            <th>状態</th>
          </tr>
        </thead>
        <tbody>
          {sessions.map(row => (
            <tr key={`${row.sessionType}-${row.sessionNo}-${row.raceNo}`}>
              <td>{sessionLabel(row)}</td>
              <td>{formatPosition(row.position)}</td>
              <td>{formatPosition(row.classPosition)}</td>
              <td>{formatPosition(row.gridPosition)}</td>
              <td>{formatPosition(row.classGridPosition)}</td>
              <td>{formatCount(row.completedLaps)}</td>
              <td>{formatLapTime(row.bestLapUs)}</td>
              <td>{row.finishTimeUs === null ? "—" : formatLapTime(row.finishTimeUs)}</td>
              <td>{row.finishStatus}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </article>
  );
}
