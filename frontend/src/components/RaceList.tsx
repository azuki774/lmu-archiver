import { Fragment } from "react";
import type { Race, SessionResult } from "../api/types";
import { formatDateTime } from "../lib/format";
import { RaceDetailCard } from "./RaceDetail";

// レース一覧。行の選択は親の共有状態に入り、詳細は選択行の直下に展開する。
// 複数のレースブロックを持つイベントは、順位を走行ごとに並べて表示する。

function raceRows(race: Race): SessionResult[] {
  return race.results
    .filter(row => row.sessionType === "race")
    .sort((a, b) => a.raceNo - b.raceNo);
}

function positionsText(race: Race, key: "position" | "classPosition"): string {
  const rows = raceRows(race);
  if (rows.length === 0) return "—";
  return rows
    .map(row => {
      const position = row[key];
      return position === null ? "DNF" : `P${position}`;
    })
    .join(", ");
}

export function RaceList({
  races,
  selectedEventId,
  onSelect,
  onClear,
}: {
  races: Race[];
  selectedEventId: string | null;
  onSelect: (race: Race) => void;
  onClear: () => void;
}) {
  return (
    <table className="data-table race-table">
      <thead>
        <tr>
          <th>日時</th>
          <th>トラック</th>
          <th>クラス</th>
          <th>種別</th>
          <th>総合順位</th>
          <th>クラス順位</th>
          <th aria-label="操作" />
        </tr>
      </thead>
      <tbody>
        {races.map(race => {
          const eventId = race.event.eventId;
          const selected = eventId === selectedEventId;
          return (
            <Fragment key={eventId}>
              <tr className={selected ? "row-selected" : undefined}>
                <td>{formatDateTime(race.event.eventAt)}</td>
                <td title={race.event.title}>{race.event.track}</td>
                <td>{race.participation.carClass}</td>
                <td>{race.event.eventType}</td>
                <td>{positionsText(race, "position")}</td>
                <td>{positionsText(race, "classPosition")}</td>
                <td className="row-actions">
                  <button type="button" onClick={() => onSelect(race)}>
                    {selected ? "選択中" : "詳細"}
                  </button>
                </td>
              </tr>
              {selected && (
                <tr className="row-detail">
                  <td colSpan={7}>
                    <RaceDetailCard race={race} onClear={onClear} />
                  </td>
                </tr>
              )}
            </Fragment>
          );
        })}
      </tbody>
    </table>
  );
}

export function RacePager({
  page,
  hasNext,
  onPageChange,
}: {
  page: number;
  hasNext: boolean;
  onPageChange: (page: number) => void;
}) {
  return (
    <div className="pager">
      <button type="button" disabled={page === 0} onClick={() => onPageChange(page - 1)}>
        前へ
      </button>
      <span>{page + 1} ページ</span>
      <button type="button" disabled={!hasNext} onClick={() => onPageChange(page + 1)}>
        次へ
      </button>
    </div>
  );
}
