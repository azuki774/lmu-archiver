import type { Period } from "./period";

// レース一覧とトラック別戦績で共通の検索条件。空の値はフィルター無効。
export interface RaceFilters {
  track: string;
  class: string;
  eventType: string;
}

export const EMPTY_FILTERS: RaceFilters = { track: "", class: "", eventType: "" };

function applyFilters(params: URLSearchParams, period: Period | null, filters: RaceFilters): void {
  if (period !== null) {
    params.set("from", period.from);
    params.set("to", period.to);
  }
  if (filters.track !== "") params.set("track", filters.track);
  if (filters.class !== "") params.set("class", filters.class);
  if (filters.eventType !== "") params.set("event-type", filters.eventType);
}

export function racesPath(period: Period | null, filters: RaceFilters, page: number, pageSize: number): string {
  const params = new URLSearchParams();
  params.set("limit", String(pageSize));
  params.set("offset", String(page * pageSize));
  applyFilters(params, period, filters);
  return `/api/v1/races?${params.toString()}`;
}

export function trackStatsPath(period: Period | null, filters: RaceFilters): string {
  const params = new URLSearchParams();
  applyFilters(params, period, filters);
  return `/api/v1/stats/tracks?${params.toString()}`;
}

export function ratingsPath(limit: number): string {
  return `/api/v1/ratings?limit=${limit}`;
}
