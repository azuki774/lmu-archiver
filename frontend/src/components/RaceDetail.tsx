import type { Race, RatingSnapshot, SessionResult, SessionType } from "../api/types";
import { formatCount, formatDateTime, formatDelta, formatLapTime, formatPosition } from "../lib/format";

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

function ratingCell(snapshot: RatingSnapshot | null, kind: "dr" | "sr"): string {
  if (snapshot === null) return "—";
  const value = kind === "dr" ? snapshot.drElo.toFixed(2) : snapshot.srRating.toFixed(2);
  const rank = kind === "dr" ? snapshot.drRank : snapshot.srRank;
  return `${value} (${rank})`;
}

function ratingDelta(
  before: RatingSnapshot | null,
  after: RatingSnapshot | null,
  kind: "dr" | "sr",
): string {
  if (before === null || after === null) return "—";
  return formatDelta(kind === "dr" ? after.drElo - before.drElo : after.srRating - before.srRating);
}

// 選択したレースのセッション詳細。referenceRating と followingRating は
// レース前後の観測（同期時点）であり、レース単位の確定値ではない。
export function RaceDetailCard({
  race,
  onClear,
}: {
  race: Race;
  onClear: () => void;
}) {
  const before = race.referenceRating;
  const after = race.followingRating;
  const sessions = [...race.results].sort(
    (a, b) =>
      SESSION_ORDER[a.sessionType] - SESSION_ORDER[b.sessionType] ||
      a.raceNo - b.raceNo ||
      a.sessionNo.localeCompare(b.sessionNo),
  );
  return (
    <article className="detail-card">
      <header className="detail-header">
        <h3>{race.event.title}</h3>
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
      <section className="rating-transition">
        <h4>DR/SR 推移</h4>
        <table className="data-table transition-table">
          <thead>
            <tr>
              <th scope="col" aria-label="種別" />
              <th scope="col">レース前観測</th>
              <th scope="col">レース後観測</th>
              <th scope="col">変化</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <th scope="row">DR</th>
              <td>{ratingCell(before, "dr")}</td>
              <td>{ratingCell(after, "dr")}</td>
              <td>{ratingDelta(before, after, "dr")}</td>
            </tr>
            <tr>
              <th scope="row">SR</th>
              <td>{ratingCell(before, "sr")}</td>
              <td>{ratingCell(after, "sr")}</td>
              <td>{ratingDelta(before, after, "sr")}</td>
            </tr>
          </tbody>
        </table>
        <p className="transition-times">
          前観測: {before === null ? "—" : formatDateTime(before.observedAt)} ・ 後観測:{" "}
          {after === null ? "—" : formatDateTime(after.observedAt)}
        </p>
        <p className="section-note">
          DR/SR は同期時点の観測値であり、前後の観測の間に別のレースが含まれることがあります。
        </p>
      </section>
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
