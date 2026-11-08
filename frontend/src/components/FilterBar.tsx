import type { FilterValues } from "../api/types";
import type { RaceFilters } from "../lib/query";

// 期間（Brush 由来）・トラック・クラス・種別の絞り込み。変更は即座に
// 親の状態へ渡され、ページ遷移なしで一覧と戦績表へ反映される。
export function FilterBar({
  values,
  valuesError,
  filters,
  onFiltersChange,
  periodLabel,
  onReset,
}: {
  values: FilterValues | null;
  valuesError: boolean;
  filters: RaceFilters;
  onFiltersChange: (filters: RaceFilters) => void;
  periodLabel: string;
  onReset: () => void;
}) {
  const select = (
    label: string,
    key: keyof RaceFilters,
    options: string[] | undefined,
  ) => (
    <label className="filter-item">
      {label}
      <select
        value={filters[key]}
        onChange={event =>
          onFiltersChange({ ...filters, [key]: event.target.value } satisfies RaceFilters)
        }
      >
        <option value="">すべて</option>
        {(options ?? []).map(option => (
          <option key={option} value={option}>
            {option}
          </option>
        ))}
      </select>
    </label>
  );
  return (
    <div className="filter-bar">
      {select("トラック", "track", values?.tracks)}
      {select("クラス", "class", values?.classes)}
      {select("種別", "eventType", values?.eventTypes)}
      <span className="period-label">期間: {periodLabel}</span>
      <button type="button" className="filter-reset" onClick={onReset}>
        リセット
      </button>
      {valuesError && (
        <span className="filter-warn" role="status">
          フィルター選択肢を取得できません
        </span>
      )}
    </div>
  );
}
