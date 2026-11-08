import { useEffect, useMemo, useState } from "react";
import type { FilterValues, Race, RatingSnapshot, TrackStats } from "./api/types";
import { FilterBar } from "./components/FilterBar";
import { RaceDetailCard } from "./components/RaceDetail";
import { RaceList, RacePager } from "./components/RaceList";
import { RatingCards } from "./components/RatingCards";
import { RatingChart } from "./components/RatingChart";
import { EmptyMessage, ErrorBanner, LoadingMessage, RefreshingBadge } from "./components/StateBanner";
import { TrackStatsTable } from "./components/TrackStatsTable";
import { brushRangeToPeriod, describePeriod, type BrushRange } from "./lib/period";
import { EMPTY_FILTERS, racesPath, ratingsPath, trackStatsPath, type RaceFilters } from "./lib/query";
import { useResource } from "./lib/useResource";

const RATING_LIMIT = 365;
const RACES_PER_PAGE = 20;
const BRUSH_COMMIT_DELAY_MS = 300;

// 1 ページのダッシュボード。共有状態（期間・フィルター・ページ・選択・比較）は
// この親コンポーネントに集約し、子へ渡す。ページ遷移は発生しない。
export default function App() {
  const ratings = useResource<RatingSnapshot[]>(ratingsPath(RATING_LIMIT));
  const filterValues = useResource<FilterValues>("/api/v1/filters");

  const [brushRange, setBrushRange] = useState<BrushRange | null>(null);
  const [committedRange, setCommittedRange] = useState<BrushRange | null>(null);
  const [chartKey, setChartKey] = useState(0);
  const [filters, setFilters] = useState<RaceFilters>(EMPTY_FILTERS);
  const [page, setPage] = useState(0);
  const [selected, setSelected] = useState<Race | null>(null);
  const [compare, setCompare] = useState<Race | null>(null);

  // Brush 操作中は選択表示を即座に更新し、操作が落ち着いた時点で
  // 期間を確定させて API 取得を行う。
  useEffect(() => {
    if (brushRange === null) return;
    const timer = window.setTimeout(() => {
      setCommittedRange(brushRange);
      setPage(0);
    }, BRUSH_COMMIT_DELAY_MS);
    return () => window.clearTimeout(timer);
  }, [brushRange]);

  const period = useMemo(
    () => brushRangeToPeriod(ratings.data ?? [], committedRange),
    [ratings.data, committedRange],
  );
  const races = useResource<Race[]>(racesPath(period, filters, page, RACES_PER_PAGE));
  const stats = useResource<TrackStats[]>(trackStatsPath(period, filters));

  const ratingSeries = ratings.data ?? [];
  const chartReady = ratings.status === "ready" && ratingSeries.length > 0;
  const racesData = races.data;
  const racesReady = racesData !== null;
  const hasNextPage = racesReady && racesData.length >= RACES_PER_PAGE;

  const handleSelect = (race: Race) => {
    setSelected(previous => (previous?.event.eventId === race.event.eventId ? null : race));
  };
  const handleCompare = (race: Race) => {
    setCompare(previous => (previous?.event.eventId === race.event.eventId ? null : race));
  };
  const handleFiltersChange = (next: RaceFilters) => {
    setFilters(next);
    setPage(0);
  };
  const handleReset = () => {
    setBrushRange(null);
    setCommittedRange(null);
    setFilters(EMPTY_FILTERS);
    setPage(0);
    setChartKey(key => key + 1);
  };

  return (
    <main className="app">
      <header className="app-header">
        <h1>LMU Archiver</h1>
      </header>

      <section className="section" aria-label="レート">
        <h2>
          レート {ratings.refreshing && <RefreshingBadge />}
        </h2>
        {ratings.status === "error" ? (
          <ErrorBanner resource={ratings} />
        ) : ratings.status === "loading" ? (
          <LoadingMessage label="レート" />
        ) : (
          <>
            <RatingCards series={ratingSeries} />
            <p className="section-note">
              レートは同期時点の観測値であり、各レース直後の確定値ではありません。
              グラフ下部のブラシで期間を選ぶと、レース一覧とトラック別戦績がその期間に絞られます。
            </p>
            {chartReady && (
              <RatingChart
                key={chartKey}
                series={ratingSeries}
                range={brushRange}
                onRangeChange={setBrushRange}
              />
            )}
          </>
        )}
      </section>

      <section className="section" aria-label="過去レース">
        <h2>
          過去レース {races.refreshing && <RefreshingBadge />}
        </h2>
        <FilterBar
          values={filterValues.data}
          valuesError={filterValues.status === "error"}
          filters={filters}
          onFiltersChange={handleFiltersChange}
          periodLabel={describePeriod(ratingSeries, brushRange)}
          onReset={handleReset}
        />
        {races.status === "error" ? (
          <ErrorBanner resource={races} />
        ) : racesData === null ? (
          <LoadingMessage label="レース" />
        ) : racesData.length === 0 ? (
          <>
            <EmptyMessage
              label={page === 0 ? "条件に一致するレースはありません。" : "このページにレースはありません。"}
            />
            {page > 0 && <RacePager page={page} hasNext={hasNextPage} onPageChange={setPage} />}
          </>
        ) : (
          <>
            <RaceList
              races={racesData}
              selectedEventId={selected?.event.eventId ?? null}
              compareEventId={compare?.event.eventId ?? null}
              onSelect={handleSelect}
              onCompare={handleCompare}
            />
            <RacePager page={page} hasNext={hasNextPage} onPageChange={setPage} />
          </>
        )}
      </section>

      {(selected !== null || compare !== null) && (
        <section className="section" aria-label="レース詳細">
          <h2>レース詳細</h2>
          <div className="detail-row">
            {selected !== null && (
              <RaceDetailCard heading="選択" race={selected} onClear={() => setSelected(null)} />
            )}
            {compare !== null && (
              <RaceDetailCard heading="比較" race={compare} onClear={() => setCompare(null)} />
            )}
          </div>
        </section>
      )}

      <section className="section" aria-label="トラック別戦績">
        <h2>
          トラック別戦績 {stats.refreshing && <RefreshingBadge />}
        </h2>
        {stats.status === "error" ? (
          <ErrorBanner resource={stats} />
        ) : stats.data === null ? (
          <LoadingMessage label="戦績" />
        ) : stats.data.length === 0 ? (
          <EmptyMessage label="条件に一致する戦績はありません。" />
        ) : (
          <>
            <TrackStatsTable stats={stats.data} />
            <p className="section-note">
              過去レースの絞り込み条件（期間・トラック・クラス・種別）に一致する全レースを集計しています（一覧の表示ページには依存しません）。
            </p>
          </>
        )}
      </section>
    </main>
  );
}
