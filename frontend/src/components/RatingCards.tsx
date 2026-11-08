import type { RatingSnapshot } from "../api/types";
import { formatDateTime, formatDelta } from "../lib/format";

// レートは同期時点の観測値。増減は前回観測との差であり、レース単位の
// 確定値ではない。比較対象がなければ「比較データなし」と表示する。
export function RatingCards({ series }: { series: RatingSnapshot[] }) {
  const latest = series.at(-1);
  if (latest === undefined) {
    return <p className="state-message">レートの観測データがありません。</p>;
  }
  const previous = series.at(-2);
  return (
    <div className="rating-cards">
      <article className="rating-card">
        <h3>DR</h3>
        <p className="rating-value">{latest.drElo.toFixed(2)}</p>
        <p className="rating-rank">{latest.drRank}</p>
        <p className="rating-observed">観測: {formatDateTime(latest.observedAt)}</p>
        <p className="rating-delta">
          前回観測比:{" "}
          {previous === undefined ? (
            <span className="no-data">比較データなし</span>
          ) : (
            formatDelta(latest.drElo - previous.drElo)
          )}
        </p>
      </article>
      <article className="rating-card">
        <h3>SR</h3>
        <p className="rating-value">{latest.srRating.toFixed(2)}</p>
        <p className="rating-rank">{latest.srRank}</p>
        <p className="rating-observed">観測: {formatDateTime(latest.observedAt)}</p>
        <p className="rating-delta">
          前回観測比:{" "}
          {previous === undefined ? (
            <span className="no-data">比較データなし</span>
          ) : (
            formatDelta(latest.srRating - previous.srRating)
          )}
        </p>
      </article>
    </div>
  );
}
