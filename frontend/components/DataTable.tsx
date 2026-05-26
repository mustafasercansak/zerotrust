"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import {
  useReactTable,
  getCoreRowModel,
  flexRender,
  type ColumnDef,
  type SortingState,
} from "@tanstack/react-table";
import { ChevronUp, ChevronDown, ChevronsUpDown, ChevronLeft, ChevronRight, Search } from "lucide-react";
import { cn } from "@/lib/utils";
import type { PageParams, PagedResult } from "@/lib/api";

export interface Column<T> {
  key: string;
  label: string;
  sortKey?: string;
  filterKey?: string;
  className?: string;
  render: (row: T) => React.ReactNode;
}

export interface Tab {
  key: string;
  label: string;
  preset?: Record<string, string>;
}

interface Props<T> {
  columns: Column<T>[];
  tabs?: Tab[];
  fetcher: (p: PageParams) => Promise<PagedResult<T>>;
  rowKey: (row: T) => string;
  pageSizeOptions?: number[];
  defaultPageSize?: number;
  defaultSortKey?: string;
  defaultSortDir?: "asc" | "desc";
  emptyMessage?: string;
}

export function DataTable<T>({
  columns,
  tabs,
  fetcher,
  rowKey,
  pageSizeOptions = [10, 25, 50],
  defaultPageSize = 25,
  defaultSortKey,
  defaultSortDir = "desc",
  emptyMessage,
}: Props<T>) {
  const t = useTranslations("common");

  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(defaultPageSize);
  const [sorting, setSorting] = useState<SortingState>(
    defaultSortKey ? [{ id: defaultSortKey, desc: defaultSortDir === "desc" }] : [],
  );
  const [rawFilters, setRawFilters] = useState<Record<string, string>>({});
  const [committed, setCommitted] = useState<Record<string, string>>({});
  const [activeTab, setActiveTab] = useState(tabs?.[0]?.key ?? "");
  const [data, setData] = useState<T[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const fetcherRef = useRef(fetcher);
  fetcherRef.current = fetcher;
  const tabsRef = useRef(tabs);
  tabsRef.current = tabs;

  useEffect(() => {
    const id = setTimeout(() => { setCommitted(rawFilters); setPage(0); }, 350);
    return () => clearTimeout(id);
  }, [rawFilters]);

  useEffect(() => {
    let cancelled = false;
    const preset = tabsRef.current?.find((tb) => tb.key === activeTab)?.preset ?? {};
    const filters = { ...preset, ...committed };
    const sort = sorting[0];

    setLoading(true);
    setError("");
    fetcherRef
      .current({
        page,
        pageSize,
        sortKey: sort?.id,
        sortDir: sort ? (sort.desc ? "desc" : "asc") : undefined,
        filters,
      })
      .then((res) => {
        if (!cancelled) { setData(res.data); setTotal(res.total); }
      })
      .catch(() => { if (!cancelled) setError(t("error")); })
      .finally(() => { if (!cancelled) setLoading(false); });

    return () => { cancelled = true; };
  }, [page, pageSize, sorting, committed, activeTab, t]);

  const tanstackCols: ColumnDef<T>[] = columns.map((col) => ({
    id: col.key,
    header: col.label,
    cell: ({ row }) => col.render(row.original),
    enableSorting: !!col.sortKey,
  }));

  const table = useReactTable({
    data,
    columns: tanstackCols,
    getCoreRowModel: getCoreRowModel(),
    manualSorting: true,
    manualPagination: true,
    manualFiltering: true,
    pageCount: Math.max(1, Math.ceil(total / pageSize)),
    state: { sorting },
    onSortingChange: (updater) => {
      const next = typeof updater === "function" ? updater(sorting) : updater;
      // Map TanStack column id back to the sortKey the API understands
      const mapped = next.map((s) => {
        const col = columns.find((c) => c.key === s.id);
        return { id: col?.sortKey ?? s.id, desc: s.desc };
      });
      setSorting(mapped);
      setPage(0);
    },
  });

  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const hasFilters = columns.some((c) => c.filterKey);
  const from = total === 0 ? 0 : page * pageSize + 1;
  const to = Math.min((page + 1) * pageSize, total);

  function handleTabChange(key: string) {
    setActiveTab(key);
    setRawFilters({});
    setCommitted({});
    setPage(0);
  }

  function handleFilter(filterKey: string, value: string) {
    setRawFilters((prev) => {
      const next = { ...prev };
      if (value) next[filterKey] = value;
      else delete next[filterKey];
      return next;
    });
  }

  function SortIcon({ colKey }: { colKey: string }) {
    const col = columns.find((c) => c.key === colKey);
    if (!col?.sortKey) return null;
    const active = sorting[0]?.id === col.sortKey;
    if (!active) return <ChevronsUpDown size={13} className="text-gray-600 shrink-0" />;
    return sorting[0].desc
      ? <ChevronDown size={13} className="text-indigo-400 shrink-0" />
      : <ChevronUp size={13} className="text-indigo-400 shrink-0" />;
  }

  return (
    <div className="flex flex-col h-full gap-3">
      {/* Tabs + page-size */}
      <div className="shrink-0 flex items-center justify-between gap-4 flex-wrap">
        <div className="flex gap-1 flex-wrap">
          {tabs?.map((tab) => (
            <button
              key={tab.key}
              onClick={() => handleTabChange(tab.key)}
              className={cn(
                "px-3.5 py-1.5 text-sm rounded-lg border transition-colors",
                activeTab === tab.key
                  ? "bg-indigo-600 border-indigo-500 text-white"
                  : "border-gray-700 text-gray-400 hover:text-white hover:border-gray-600 bg-transparent",
              )}
            >
              {tab.label}
            </button>
          ))}
        </div>

        <div className="flex items-center gap-2 shrink-0">
          <span className="text-xs text-gray-500">{t("rowsPerPage")}</span>
          <select
            value={pageSize}
            onChange={(e) => { setPageSize(Number(e.target.value)); setPage(0); }}
            className="bg-gray-900 border border-gray-700 text-gray-300 rounded-lg px-2.5 py-1.5 text-xs focus:outline-none focus:border-indigo-500 cursor-pointer"
          >
            {pageSizeOptions.map((n) => <option key={n} value={n}>{n}</option>)}
          </select>
        </div>
      </div>

      {error && <p className="shrink-0 text-red-400 text-sm">{error}</p>}

      {/* Table — flex-1 lets it fill remaining height; inner div scrolls */}
      <div className="flex-1 min-h-0 rounded-xl border border-gray-800 overflow-hidden">
        <div className="overflow-auto h-full">
        <table className="w-full text-sm">
          <thead className="sticky top-0 z-10">
            {table.getHeaderGroups().map((hg) => (
              <tr key={hg.id} className="border-b border-gray-800 bg-gray-900">
                {hg.headers.map((header) => {
                  const col = columns.find((c) => c.key === header.id);
                  const sortable = !!col?.sortKey;
                  return (
                    <th
                      key={header.id}
                      onClick={sortable ? () => {
                        const cur = sorting[0];
                        if (cur?.id === col!.sortKey) {
                          setSorting([{ id: col!.sortKey!, desc: !cur.desc }]);
                        } else {
                          setSorting([{ id: col!.sortKey!, desc: false }]);
                        }
                        setPage(0);
                      } : undefined}
                      className={cn(
                        "px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider",
                        sortable && "cursor-pointer select-none hover:text-gray-200",
                        col?.className,
                      )}
                    >
                      <span className="inline-flex items-center gap-1.5">
                        {flexRender(header.column.columnDef.header, header.getContext())}
                        <SortIcon colKey={header.id} />
                      </span>
                    </th>
                  );
                })}
              </tr>
            ))}

            {hasFilters && (
              <tr className="border-b border-gray-800/60 bg-gray-900/70">
                {columns.map((col) => (
                  <td key={col.key} className="px-3 py-2">
                    {col.filterKey ? (
                      <div className="relative">
                        <Search size={11} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-gray-600 pointer-events-none" />
                        <input
                          type="text"
                          value={rawFilters[col.filterKey] ?? ""}
                          onChange={(e) => handleFilter(col.filterKey!, e.target.value)}
                          placeholder="…"
                          className="w-full bg-gray-800/80 border border-gray-700/60 rounded-md pl-7 pr-2 py-1 text-xs text-white placeholder-gray-600 focus:outline-none focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500/30 transition-colors"
                        />
                      </div>
                    ) : null}
                  </td>
                ))}
              </tr>
            )}
          </thead>

          <tbody>
            {loading ? (
              <tr>
                <td colSpan={columns.length} className="px-4 py-10 text-center text-gray-500 text-sm">
                  <span className="inline-flex items-center gap-2">
                    <span className="w-4 h-4 border-2 border-gray-700 border-t-indigo-500 rounded-full animate-spin" />
                    {t("loading")}
                  </span>
                </td>
              </tr>
            ) : table.getRowModel().rows.length === 0 ? (
              <tr>
                <td colSpan={columns.length} className="px-4 py-10 text-center text-gray-500 text-sm">
                  {emptyMessage ?? "—"}
                </td>
              </tr>
            ) : (
              table.getRowModel().rows.map((row) => (
                <tr
                  key={rowKey(row.original)}
                  className="border-b border-gray-800/50 last:border-0 bg-gray-950 hover:bg-gray-900/60 transition-colors"
                >
                  {row.getVisibleCells().map((cell) => {
                    const col = columns.find((c) => c.key === cell.column.id);
                    return (
                      <td key={cell.id} className={cn("px-4 py-3", col?.className)}>
                        {flexRender(cell.column.columnDef.cell, cell.getContext())}
                      </td>
                    );
                  })}
                </tr>
              ))
            )}
          </tbody>
        </table>
        </div>
      </div>

      {/* Pagination */}
      <div className="shrink-0 flex items-center justify-between gap-4">
        <span className="text-xs text-gray-500">
          {total > 0 && t("showingOf", { from, to, total })}
        </span>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setPage((p) => p - 1)}
            disabled={page === 0}
            className={cn(
              "p-1.5 rounded-lg border transition-colors",
              page === 0
                ? "border-gray-800 text-gray-700 cursor-not-allowed"
                : "border-gray-700 text-gray-400 hover:text-white hover:border-gray-500",
            )}
          >
            <ChevronLeft size={16} />
          </button>
          <span className="text-xs text-gray-500 min-w-[4.5rem] text-center">
            {t("pageOf", { page: page + 1, total: totalPages })}
          </span>
          <button
            onClick={() => setPage((p) => p + 1)}
            disabled={page >= totalPages - 1}
            className={cn(
              "p-1.5 rounded-lg border transition-colors",
              page >= totalPages - 1
                ? "border-gray-800 text-gray-700 cursor-not-allowed"
                : "border-gray-700 text-gray-400 hover:text-white hover:border-gray-500",
            )}
          >
            <ChevronRight size={16} />
          </button>
        </div>
      </div>
    </div>
  );
}
