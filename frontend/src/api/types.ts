// API レスポンスの型。JSON の形は internal/storage/types.go の定義と一致する。
// 欠損値は null で届く。ゼロへの置換は行わない。

export interface Event {
  eventId: string;
  seriesId: string;
  eventAt: string;
  title: string;
  eventType: string;
  tier: string;
  track: string;
  trackScene: string;
  splitNo: string;
  official: boolean;
  teamEvent: boolean;
  rawPath: string;
  importedAt: string;
}

export interface Participation {
  eventId: string;
  playerId: number;
  name: string;
  carClass: string;
  drRank: string | null;
  drTier: number | null;
  drProgress: number | null;
  srRank: string | null;
  srTier: number | null;
  srProgress: number | null;
  drAdjustment: number | null;
  srAdjustment: number | null;
  jokerPlayed: boolean;
}

export type SessionType = "practice" | "qualifying" | "warmup" | "race";

export interface SessionResult {
  eventId: string;
  playerId: number;
  sessionType: SessionType;
  sessionNo: string;
  raceNo: number;
  position: number | null;
  classPosition: number | null;
  gridPosition: number | null;
  classGridPosition: number | null;
  carType: string;
  carClass: string;
  carNumber: string;
  teamName: string;
  completedLaps: number | null;
  bestLapUs: number | null;
  finishTimeUs: number | null;
  finishStatus: string;
}

export interface RatingSnapshot {
  snapshotId: number;
  playerId: number;
  observedAt: string;
  source: string;
  rawPath: string;
  drElo: number;
  drRank: string;
  drTier: number;
  drProgress: number;
  srRating: number;
  srRank: string;
  srTier: number;
  srProgress: number;
}

export interface Race {
  event: Event;
  participation: Participation;
  results: SessionResult[];
  referenceRating: RatingSnapshot | null;
}

export interface PositionStats {
  wins: number;
  podiums: number;
  avgPosition: number | null;
  bestPosition: number | null;
}

export interface TrackStats {
  track: string;
  races: number;
  starts: number;
  overall: PositionStats;
  class: PositionStats;
  bestLapUs: number | null;
}

export interface FilterValues {
  tracks: string[];
  classes: string[];
  eventTypes: string[];
}
