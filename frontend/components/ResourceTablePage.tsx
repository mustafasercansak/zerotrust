"use client";

import { useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Tab from "@mui/material/Tab";
import Tabs from "@mui/material/Tabs";
import {
  DataGrid,
  type GridColDef,
  type GridFilterModel,
  type GridPaginationModel,
  type GridSortModel,
  type GridValidRowModel,
} from "@mui/x-data-grid";
import { DashboardPage } from "@/components/DashboardPage";
import type { PageParams, PagedResult } from "@/lib/api";

interface ResourceTablePageProps<T extends GridValidRowModel> {
  columns: GridColDef<T>[];
  fetcher: (p: PageParams) => Promise<PagedResult<T>>;
  getRowId?: (row: T) => string;
  tabs?: Array<{
    key: string;
    label: string;
    preset?: Record<string, string>;
  }>;
  action?: React.ReactNode;
  accessDenied?: boolean;
  accessDeniedMessage?: React.ReactNode;
  emptyMessage?: string;
  defaultSortKey?: string;
  defaultSortDir?: "asc" | "desc";
  defaultPageSize?: number;
  pageSizeOptions?: number[];
}

export function ResourceTablePage<T extends GridValidRowModel>({
  columns,
  fetcher,
  getRowId,
  tabs,
  action,
  accessDenied,
  accessDeniedMessage,
  emptyMessage,
  defaultSortKey,
  defaultSortDir = "desc",
  defaultPageSize = 25,
  pageSizeOptions = [10, 25, 50],
}: ResourceTablePageProps<T>) {
  const t = useTranslations("common");

  const [rows, setRows] = useState<T[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [activeTab, setActiveTab] = useState(tabs?.[0]?.key ?? "");
  const [paginationModel, setPaginationModel] = useState<GridPaginationModel>({
    page: 0,
    pageSize: defaultPageSize,
  });
  const [sortModel, setSortModel] = useState<GridSortModel>(
    defaultSortKey ? [{ field: defaultSortKey, sort: defaultSortDir }] : [],
  );
  const [filterModel, setFilterModel] = useState<GridFilterModel>({ items: [] });

  const filters = useMemo(() => {
    const preset = tabs?.find((tab) => tab.key === activeTab)?.preset ?? {};
    const next: Record<string, string> = {};
    for (const item of filterModel.items) {
      if (item.field && item.value) {
        next[item.field] = String(item.value);
      }
    }
    return { ...preset, ...next };
  }, [activeTab, filterModel, tabs]);

  useEffect(() => {
    if (accessDenied) return;

    let cancelled = false;
    const sort = sortModel[0];

    async function load() {
      setLoading(true);
      setError("");
      try {
        const res = await fetcher({
          page: paginationModel.page,
          pageSize: paginationModel.pageSize,
          sortKey: sort?.field,
          sortDir: sort?.sort ?? undefined,
          filters,
        });
        if (!cancelled) {
          setRows(res.data);
          setTotal(res.total);
        }
      } catch {
        if (!cancelled) setError(t("error"));
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    void Promise.resolve().then(load);

    return () => {
      cancelled = true;
    };
  }, [accessDenied, fetcher, filters, paginationModel, sortModel, t]);

  return (
    <DashboardPage
      action={action}
      accessDenied={accessDenied}
      accessDeniedMessage={accessDeniedMessage}
    >
      {tabs?.length ? (
        <Tabs
          value={activeTab}
          onChange={(_, value: string) => {
            setActiveTab(value);
            setFilterModel({ items: [] });
            setPaginationModel((prev) => ({ ...prev, page: 0 }));
          }}
          sx={{ minHeight: 36, flexShrink: 0 }}
        >
          {tabs.map((tab) => (
            <Tab
              key={tab.key}
              value={tab.key}
              label={tab.label}
              sx={{ minHeight: 36, py: 0.75 }}
            />
          ))}
        </Tabs>
      ) : null}
      {error ? (
        <Alert severity="error" sx={{ flexShrink: 0 }}>
          {error}
        </Alert>
      ) : null}
      <Box sx={{ flex: 1, minHeight: 0 }}>
        <DataGrid
          rows={rows}
          columns={columns}
          getRowId={getRowId}
          rowCount={total}
          loading={loading}
          paginationMode="server"
          sortingMode="server"
          filterMode="server"
          paginationModel={paginationModel}
          onPaginationModelChange={setPaginationModel}
          sortModel={sortModel}
          onSortModelChange={(model) => {
            setSortModel(model);
            setPaginationModel((prev) => ({ ...prev, page: 0 }));
          }}
          filterModel={filterModel}
          onFilterModelChange={(model) => {
            setFilterModel(model);
            setPaginationModel((prev) => ({ ...prev, page: 0 }));
          }}
          pageSizeOptions={pageSizeOptions}
          disableRowSelectionOnClick
          localeText={{
            noRowsLabel: emptyMessage ?? "—",
          }}
          sx={{
            borderColor: "divider",
            bgcolor: "background.paper",
            "& .MuiDataGrid-columnHeaders": {
              bgcolor: "#111827",
            },
            "& .MuiDataGrid-row:hover": {
              bgcolor: "rgba(99, 102, 241, 0.08)",
            },
          }}
        />
      </Box>
    </DashboardPage>
  );
}
