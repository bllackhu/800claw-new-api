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

export function resolvePoolUsageLocale(language) {
  return language?.startsWith('zh') ? 'zh-CN' : 'en-US';
}

export function formatPoolUsageNumber(value, locale, fallback = '--') {
  const n = Number(value);
  if (!Number.isFinite(n)) return fallback;
  return n.toLocaleString(locale === 'zh-CN' ? 'zh-CN' : 'en-US');
}

export function formatTokensKbMb(value, locale, emDash, tokenUnitLabel) {
  if (value == null) return emDash;
  const n = Number(value);
  if (!Number.isFinite(n)) return emDash;
  const kb = 1000;
  const mb = kb * 1000;
  if (n >= mb) return `${(n / mb).toFixed(1)} MB`;
  if (n >= kb) return `${Math.floor(n / kb)} KB`;
  return `${formatPoolUsageNumber(n, locale, emDash)} ${tokenUnitLabel}`;
}

export function formatPoolUsageCount(metric, locale, unavailable = '--') {
  if (!metric) return unavailable;
  if (metric.available && metric.count != null) {
    const limit = Number(metric.limit_count);
    if (Number.isFinite(limit)) {
      return `${formatPoolUsageNumber(metric.count, locale)}/${formatPoolUsageNumber(limit, locale)}`;
    }
    return formatPoolUsageNumber(metric.count, locale);
  }
  return unavailable;
}

export function formatPoolTokenLine(row, t, locale, emDash, unavailable) {
  if (!row) return unavailable;
  const tokenUnit = t('Token');
  return `${t('输入')} ${formatTokensKbMb(row.prompt_tokens, locale, emDash, tokenUnit)} · ${t('输出')} ${formatTokensKbMb(row.completion_tokens, locale, emDash, tokenUnit)} · ${t('总计')} ${formatTokensKbMb(row.total_tokens, locale, emDash, tokenUnit)}`;
}

export function getPoolUsageReasonText(reason, t) {
  switch (reason) {
    case 'no_resolved_pool':
      return t('未解析到池');
    case 'redis_required':
      return t('需要 Redis 才能统计');
    case 'window_not_retained':
      return t('当前池未保留该时间窗口');
    case 'token_scope_not_enabled':
      return t('当前池未启用令牌维度统计');
    case 'user_scope_only':
      return t('当前池仅按用户维度统计');
    default:
      return t('暂无可用数据');
  }
}

export function hasPoolLlmTokenUsage(llmTokenUsage) {
  if (!llmTokenUsage) return false;
  const byWindow = llmTokenUsage.by_window || {};
  const hasWindow = Object.values(byWindow).some((row) => {
    if (!row) return false;
    return (
      Number(row.prompt_tokens) > 0 ||
      Number(row.completion_tokens) > 0 ||
      Number(row.total_tokens) > 0
    );
  });
  const lifetime = llmTokenUsage.lifetime;
  const hasLifetime =
    lifetime &&
    (Number(lifetime.prompt_tokens) > 0 ||
      Number(lifetime.completion_tokens) > 0 ||
      Number(lifetime.total_tokens) > 0);
  return hasWindow || hasLifetime;
}
