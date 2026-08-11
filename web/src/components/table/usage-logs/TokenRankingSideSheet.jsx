/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useEffect, useMemo, useState } from 'react';
import {
  Button,
  DatePicker,
  Empty,
  Input,
  SideSheet,
  Space,
  Spin,
  Table,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { VChart } from '@visactor/react-vchart';
import { initVChartSemiTheme } from '@visactor/vchart-semi-theme';
import { IconSearch, IconRefresh } from '@douyinfe/semi-icons';
import { renderNumber } from '../../../helpers';
import { DATE_RANGE_PRESETS } from '../../../constants/console.constants';

const { Text } = Typography;

const USER_COLORS = [
  '#3b82f6',
  '#ef4444',
  '#10b981',
  '#f59e0b',
  '#8b5cf6',
  '#ec4899',
  '#06b6d4',
  '#f97316',
  '#6366f1',
  '#14b8a6',
];

const toUnixSeconds = (value) => {
  const ts = Date.parse(value);
  return Number.isNaN(ts) ? 0 : ts / 1000;
};

const TokenRankingSideSheet = ({
  t,
  showTokenRanking,
  setShowTokenRanking,
  tokenRanking,
  tokenRankingItems,
  tokenRankingTotal,
  tokenRankingLoading,
  loadTokenRanking,
  getFormValues,
  formInitValues,
}) => {
  const [localFilters, setLocalFilters] = useState(null);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  const buildLocalFilters = () => {
    let pageValues;
    try {
      pageValues = getFormValues();
    } catch (e) {
      pageValues = {};
    }
    const fallbackRange = Array.isArray(formInitValues?.dateRange)
      ? formInitValues.dateRange
      : [];
    return {
      dateRange: [
        pageValues.start_timestamp || fallbackRange[0] || '',
        pageValues.end_timestamp || fallbackRange[1] || '',
      ],
      token_name: pageValues.token_name || '',
      model_name: pageValues.model_name || '',
      username: pageValues.username || '',
      channel: pageValues.channel || '',
      group: pageValues.group || '',
    };
  };

  const toParams = (filters) => {
    const range = Array.isArray(filters.dateRange) ? filters.dateRange : [];
    return {
      start_timestamp: toUnixSeconds(range[0]),
      end_timestamp: toUnixSeconds(range[1]),
      token_name: filters.token_name || '',
      model_name: filters.model_name || '',
      username: filters.username || '',
      channel: filters.channel || '',
      group: filters.group || '',
    };
  };

  useEffect(() => {
    initVChartSemiTheme({ isWatchingThemeSwitch: true });
  }, []);

  useEffect(() => {
    if (!showTokenRanking) {
      return;
    }
    const filters = buildLocalFilters();
    setLocalFilters(filters);
    setPage(1);
    setPageSize(20);
    loadTokenRanking(toParams(filters), 1, 20);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [showTokenRanking]);

  const handleQuery = () => {
    if (!localFilters) return;
    setPage(1);
    loadTokenRanking(toParams(localFilters), 1, pageSize);
  };

  const handleRefresh = () => {
    if (!localFilters) return;
    loadTokenRanking(toParams(localFilters), page, pageSize);
  };

  const handleFieldChange = (field, value) => {
    setLocalFilters((prev) => ({ ...prev, [field]: value }));
  };

  const handlePageChange = (nextPage) => {
    if (!localFilters) return;
    setPage(nextPage);
    loadTokenRanking(toParams(localFilters), nextPage, pageSize);
  };

  const handlePageSizeChange = (size) => {
    if (!localFilters) return;
    setPageSize(size);
    setPage(1);
    loadTokenRanking(toParams(localFilters), 1, size);
  };

  const chartValues = useMemo(() => {
    return tokenRanking.map((item) => {
      const name = item.token_name || t('未命名令牌');
      return {
        Token: name,
        RequestCount: Number(item.request_count || 0),
      };
    });
  }, [tokenRanking, t]);

  const totalRequests = Number(tokenRankingTotal.request_count || 0);

  const chartSpec = useMemo(() => {
    return {
      type: 'bar',
      data: [{ id: 'tokenRankData', values: chartValues }],
      xField: 'RequestCount',
      yField: 'Token',
      seriesField: 'Token',
      direction: 'horizontal',
      legends: { visible: false },
      title: {
        visible: true,
        text: t('令牌请求次数排行'),
        subtext: `${t('总计')}：${renderNumber(totalRequests)}`,
      },
      bar: {
        state: {
          hover: { stroke: '#000', lineWidth: 1 },
        },
      },
      label: {
        visible: true,
        position: 'outside',
        formatMethod: (value) => renderNumber(Number(value)),
      },
      axes: [
        {
          orient: 'left',
          type: 'band',
          label: { visible: true },
        },
        {
          orient: 'bottom',
          type: 'linear',
          visible: false,
        },
      ],
      tooltip: {
        mark: {
          content: [
            {
              key: (datum) => datum['Token'],
              value: (datum) => renderNumber(Number(datum['RequestCount'])),
            },
          ],
        },
      },
      color: { type: 'ordinal', range: USER_COLORS },
    };
  }, [chartValues, totalRequests, t]);

  const tableData = useMemo(() => {
    return (tokenRankingItems || []).map((item, idx) => ({
      key: `${item.token_id}-${idx}`,
      rank: (page - 1) * pageSize + idx + 1,
      token_name: item.token_name,
      request_count: Number(item.request_count || 0),
      share:
        totalRequests > 0
          ? `${((item.request_count / totalRequests) * 100).toFixed(2)}%`
          : '-',
      total_tokens:
        Number(item.prompt_tokens || 0) + Number(item.completion_tokens || 0),
      model_count: Number(item.model_count || 0),
      models: item.models || [],
    }));
  }, [tokenRankingItems, totalRequests, page, pageSize]);

  const MAX_MODELS_SHOWN = 3;

  const renderModelList = (models, t) => {
    if (!Array.isArray(models) || models.length === 0) {
      return '-';
    }
    const shown = models.slice(0, MAX_MODELS_SHOWN);
    const rest = models.slice(MAX_MODELS_SHOWN);
    const line = (m) => (
      <div
        key={m.model_name}
        className='flex items-center justify-between gap-2'
      >
        <Text
          size='small'
          ellipsis={{ showTooltip: true, pos: 'end' }}
          style={{ maxWidth: 130 }}
        >
          {m.model_name}
        </Text>
        <Text size='small'>{renderNumber(m.request_count)}</Text>
      </div>
    );
    return (
      <div className='flex flex-col gap-0.5'>
        {shown.map(line)}
        {rest.length > 0 && (
          <Tooltip
            position='left'
            content={
              <div className='flex flex-col gap-0.5'>
                {rest.map((m) => (
                  <div key={m.model_name}>
                    {m.model_name}（{renderNumber(m.request_count)}）
                  </div>
                ))}
              </div>
            }
          >
            <span
              className='cursor-pointer text-semi-color-primary'
              style={{ fontSize: 12 }}
            >
              {t('另有 {{count}} 个模型', { count: rest.length })}
            </span>
          </Tooltip>
        )}
      </div>
    );
  };

  const columns = useMemo(() => {
    return [
      {
        title: t('排名'),
        dataIndex: 'rank',
        width: 64,
        render: (value) => {
          const colorMap = { 1: '#f59e0b', 2: '#94a3b8', 3: '#d97706' };
          const color = colorMap[value];
          return (
            <Text strong style={color ? { color } : undefined}>
              {value}
            </Text>
          );
        },
      },
      {
        title: t('令牌名称'),
        dataIndex: 'token_name',
        render: (value) => value || '-',
      },
      {
        title: t('请求次数'),
        dataIndex: 'request_count',
        width: 96,
        render: (value) => renderNumber(value),
      },
      {
        title: t('模型数'),
        dataIndex: 'model_count',
        width: 220,
        render: (value, record) => renderModelList(record?.models, t),
      },
      {
        title: t('占比'),
        dataIndex: 'share',
        width: 88,
      },
      {
        title: t('Token 使用量'),
        dataIndex: 'total_tokens',
        width: 120,
        render: (value) => renderNumber(value),
      },
    ];
  }, [t]);

  const filterInput = (field, placeholder) => (
    <Input
      value={localFilters?.[field] || ''}
      onChange={(value) => handleFieldChange(field, value)}
      prefix={<IconSearch />}
      placeholder={placeholder}
      showClear
      size='small'
    />
  );

  return (
    <SideSheet
      visible={showTokenRanking}
      onCancel={() => setShowTokenRanking(false)}
      title={t('令牌请求统计')}
      width={960}
      footer={null}
      closeIcon={null}
    >
      <div className='flex flex-col gap-4 p-1'>
        {/* Filter bar */}
        <div className='rounded-xl border border-semi-color-border p-3 flex flex-col gap-3'>
          <DatePicker
            value={localFilters?.dateRange || []}
            onChange={(value) => handleFieldChange('dateRange', value)}
            type='dateTimeRange'
            placeholder={[t('开始时间'), t('结束时间')]}
            showClear
            pure
            size='small'
            inputReadOnly
            presets={DATE_RANGE_PRESETS.map((preset) => ({
              text: t(preset.text),
              start: preset.start(),
              end: preset.end(),
            }))}
          />
          <div className='grid grid-cols-1 sm:grid-cols-2 gap-2'>
            {filterInput('token_name', t('令牌名称'))}
            {filterInput('model_name', t('模型名称'))}
            {filterInput('username', t('用户名称'))}
            {filterInput('channel', t('渠道 ID'))}
            {filterInput('group', t('分组'))}
          </div>
          <Space>
            <Button
              theme='solid'
              type='primary'
              size='small'
              onClick={handleQuery}
            >
              {t('查询')}
            </Button>
            <Button
              size='small'
              type='tertiary'
              icon={<IconRefresh />}
              onClick={handleRefresh}
            >
              {t('刷新统计')}
            </Button>
          </Space>
        </div>

        {/* Summary cards */}
        <div className='flex flex-wrap gap-3'>
          <div className='flex-1 min-w-[120px] rounded-xl border border-semi-color-border p-3'>
            <Text type='tertiary' size='small'>
              {t('总请求数')}
            </Text>
            <div className='text-xl font-semibold mt-1'>
              {renderNumber(totalRequests)}
            </div>
          </div>
          <div className='flex-1 min-w-[120px] rounded-xl border border-semi-color-border p-3'>
            <Text type='tertiary' size='small'>
              {t('涉及令牌数')}
            </Text>
            <div className='text-xl font-semibold mt-1'>
              {renderNumber(tokenRankingTotal.token_count)}
            </div>
          </div>
        </div>

        {/* Chart */}
        <Spin spinning={tokenRankingLoading} tip={t('加载中...')}>
          <div className='rounded-xl border border-semi-color-border p-2'>
            {chartValues.length > 0 ? (
              <VChart
                spec={chartSpec}
                option={{ mode: 'desktop-browser' }}
                style={{ height: 360 }}
              />
            ) : (
              !tokenRankingLoading && (
                <Empty
                  image={
                    <IllustrationNoResult style={{ width: 120, height: 120 }} />
                  }
                  darkModeImage={
                    <IllustrationNoResultDark
                      style={{ width: 120, height: 120 }}
                    />
                  }
                  description={t('暂无可展示数据')}
                  style={{ padding: 30 }}
                />
              )
            )}
          </div>
        </Spin>

        {/* Ranking table */}
        <div className='rounded-xl border border-semi-color-border p-2'>
          <Table
            columns={columns}
            dataSource={tableData}
            size='small'
            loading={tokenRankingLoading}
            pagination={{
              currentPage: page,
              pageSize: pageSize,
              total: Number(tokenRankingTotal.token_count || 0),
              pageSizeOptions: [10, 20, 50, 100],
              showSizeChanger: true,
              onPageSizeChange: handlePageSizeChange,
              onPageChange: handlePageChange,
            }}
            empty={
              <Empty
                image={
                  <IllustrationNoResult style={{ width: 120, height: 120 }} />
                }
                darkModeImage={
                  <IllustrationNoResultDark
                    style={{ width: 120, height: 120 }}
                  />
                }
                description={t('暂无可展示数据')}
                style={{ padding: 30 }}
              />
            }
          />
        </div>
      </div>
    </SideSheet>
  );
};

export default TokenRankingSideSheet;
