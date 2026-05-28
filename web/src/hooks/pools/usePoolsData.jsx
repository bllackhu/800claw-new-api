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

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Button, Modal, Space, Tag, Typography } from '@douyinfe/semi-ui';
import { API, showError, showSuccess } from '../../helpers';
import { useTranslation } from 'react-i18next';

const PAGE_SIZE = 20;

const getErrorMessage = (error, fallback) =>
  error?.response?.data?.message || error?.message || fallback;

/** Keep raw string while typing; parse only when saving (see PoolFormSideSheet). */
const formatMonthlyPriceCnyInput = (value) => {
  if (value === null || value === undefined || value === '') {
    return '0';
  }
  return String(value);
};

/** Admin token subscription: period end uses 23:59:59 on the chosen calendar day (local). */
export const toPeriodEndDateEndOfDay = (dateLike) => {
  const d = dateLike ? new Date(dateLike) : new Date();
  if (Number.isNaN(d.getTime())) {
    const fallback = new Date();
    fallback.setHours(23, 59, 59, 0);
    return fallback;
  }
  const out = new Date(d);
  out.setHours(23, 59, 59, 0);
  return out;
};

const parseMonthlyPriceCny = (form) => {
  const raw =
    form.monthly_price_cny_input !== undefined &&
    form.monthly_price_cny_input !== null
      ? String(form.monthly_price_cny_input).trim()
      : String(form.monthly_price_cny ?? '').trim();
  if (raw === '') {
    return 0;
  }
  const n = Number.parseFloat(raw);
  return Number.isFinite(n) && n >= 0 ? n : 0;
};

const boolTag = (value) =>
  value ? <Tag color='green'>Enabled</Tag> : <Tag color='orange'>Disabled</Tag>;

export const usePoolsData = () => {
  const { t } = useTranslation();

  const [activeTab, setActiveTab] = useState('binding');

  const [poolItems, setPoolItems] = useState([]);
  const [poolTotal, setPoolTotal] = useState(0);
  const [poolPage, setPoolPage] = useState(1);
  const [poolLoading, setPoolLoading] = useState(false);
  const [poolForm, setPoolForm] = useState({
    id: 0,
    name: '',
    description: '',
    status: 1,
    monthly_price_cny: 0,
    monthly_price_cny_input: '0',
    billing_currency: 'CNY',
    billing_period_seconds: 30 * 24 * 3600,
  });
  const [showPoolForm, setShowPoolForm] = useState(false);

  const [subOrderItems, setSubOrderItems] = useState([]);
  const [subOrderTotal, setSubOrderTotal] = useState(0);
  const [subOrderPage, setSubOrderPage] = useState(1);
  const [subOrderLoading, setSubOrderLoading] = useState(false);

  const [tokenSubItems, setTokenSubItems] = useState([]);
  const [tokenSubTotal, setTokenSubTotal] = useState(0);
  const [tokenSubPage, setTokenSubPage] = useState(1);
  const [tokenSubLoading, setTokenSubLoading] = useState(false);
  const [tokenSubFilterTokenId, setTokenSubFilterTokenId] = useState('');
  const [tokenSubFilterPoolId, setTokenSubFilterPoolId] = useState('');
  const [tokenSubForm, setTokenSubForm] = useState({
    id: 0,
    token_id: '',
    pool_id: '',
    period_end_date: null,
  });
  const [showTokenSubForm, setShowTokenSubForm] = useState(false);

  const [channelItems, setChannelItems] = useState([]);
  const [channelTotal, setChannelTotal] = useState(0);
  const [channelPage, setChannelPage] = useState(1);
  const [channelLoading, setChannelLoading] = useState(false);
  const [channelForm, setChannelForm] = useState({
    id: 0,
    pool_id: '',
    channel_id: '',
    weight: 0,
    priority: 0,
    enabled: true,
  });
  const [showChannelForm, setShowChannelForm] = useState(false);
  const [channelPoolFilter, setChannelPoolFilter] = useState('');

  const [policyItems, setPolicyItems] = useState([]);
  const [policyTotal, setPolicyTotal] = useState(0);
  const [policyPage, setPolicyPage] = useState(1);
  const [policyLoading, setPolicyLoading] = useState(false);
  const [policyForm, setPolicyForm] = useState({
    id: 0,
    pool_id: '',
    metric: 'request_count',
    scope_type: 'token',
    window_seconds: 5 * 3600,
    limit_count: 1000,
    enabled: true,
  });
  const [showPolicyForm, setShowPolicyForm] = useState(false);
  const [policyPoolFilter, setPolicyPoolFilter] = useState('');

  const [bindingItems, setBindingItems] = useState([]);
  const [bindingTotal, setBindingTotal] = useState(0);
  const [bindingPage, setBindingPage] = useState(1);
  const [bindingLoading, setBindingLoading] = useState(false);
  const [bindingForm, setBindingForm] = useState({
    id: 0,
    binding_type: 'token',
    binding_value: '',
    pool_id: '',
    priority: 0,
    enabled: true,
  });
  const [showBindingForm, setShowBindingForm] = useState(false);
  const [bindingTypeFilter, setBindingTypeFilter] = useState('');
  const [bindingValueFilter, setBindingValueFilter] = useState('');
  const [bindingNameFilter, setBindingNameFilter] = useState('');

  const [usageLoading, setUsageLoading] = useState(false);
  const [usageQuery, setUsageQuery] = useState({
    pool_id: '',
    scope_type: 'token',
    scope_id: '',
    window: '5h',
  });
  const [usageResult, setUsageResult] = useState(null);
  const [selfUsageResult, setSelfUsageResult] = useState(null);

  const loadPools = useCallback(
    async (targetPage = poolPage) => {
      setPoolLoading(true);
      try {
        const res = await API.get(
          `/api/pool/?p=${targetPage}&page_size=${PAGE_SIZE}`,
        );
        const { success, message, data } = res.data;
        if (!success) {
          showError(message || t('Failed to load pools'));
          return;
        }
        setPoolItems(data?.items || []);
        setPoolTotal(data?.total || 0);
        setPoolPage(data?.page || targetPage);
      } catch (error) {
        showError(getErrorMessage(error, t('Failed to load pools')));
      } finally {
        setPoolLoading(false);
      }
    },
    [poolPage, t],
  );

  const loadPoolChannels = useCallback(
    async (
      targetPage = channelPage,
      overrides = {},
    ) => {
      setChannelLoading(true);
      try {
        const effectivePoolFilter =
          overrides.poolFilter !== undefined
            ? overrides.poolFilter
            : channelPoolFilter;
        const poolQuery = effectivePoolFilter
          ? `&pool_id=${effectivePoolFilter}`
          : '';
        const res = await API.get(
          `/api/pool/channel?p=${targetPage}&page_size=${PAGE_SIZE}${poolQuery}`,
        );
        const { success, message, data } = res.data;
        if (!success) {
          showError(message || t('Failed to load pool channels'));
          return;
        }
        setChannelItems(data?.items || []);
        setChannelTotal(data?.total || 0);
        setChannelPage(data?.page || targetPage);
      } catch (error) {
        showError(getErrorMessage(error, t('Failed to load pool channels')));
      } finally {
        setChannelLoading(false);
      }
    },
    [channelPage, channelPoolFilter, t],
  );

  const loadPolicies = useCallback(
    async (
      targetPage = policyPage,
      overrides = {},
    ) => {
      setPolicyLoading(true);
      try {
        const effectivePoolFilter =
          overrides.poolFilter !== undefined
            ? overrides.poolFilter
            : policyPoolFilter;
        const poolQuery = effectivePoolFilter
          ? `&pool_id=${effectivePoolFilter}`
          : '';
        const res = await API.get(
          `/api/pool/policy?p=${targetPage}&page_size=${PAGE_SIZE}${poolQuery}`,
        );
        const { success, message, data } = res.data;
        if (!success) {
          showError(message || t('Failed to load pool policies'));
          return;
        }
        setPolicyItems(data?.items || []);
        setPolicyTotal(data?.total || 0);
        setPolicyPage(data?.page || targetPage);
      } catch (error) {
        showError(getErrorMessage(error, t('Failed to load pool policies')));
      } finally {
        setPolicyLoading(false);
      }
    },
    [policyPage, policyPoolFilter, t],
  );

  const loadBindings = useCallback(
    async (
      targetPage = bindingPage,
      overrides = {},
    ) => {
      setBindingLoading(true);
      try {
        const effectiveBindingType =
          overrides.bindingType !== undefined
            ? overrides.bindingType
            : bindingTypeFilter;
        const effectiveBindingValue =
          overrides.bindingValue !== undefined
            ? overrides.bindingValue
            : bindingValueFilter;
        const effectiveBindingName =
          overrides.bindingName !== undefined
            ? overrides.bindingName
            : bindingNameFilter;

        const typeQuery = effectiveBindingType
          ? `&binding_type=${encodeURIComponent(effectiveBindingType)}`
          : '';
        const valueQuery = effectiveBindingValue
          ? `&binding_value=${encodeURIComponent(effectiveBindingValue)}`
          : '';
        const nameQuery = effectiveBindingName
          ? `&binding_name=${encodeURIComponent(effectiveBindingName)}`
          : '';
        const res = await API.get(
          `/api/pool/binding?p=${targetPage}&page_size=${PAGE_SIZE}${typeQuery}${valueQuery}${nameQuery}`,
        );
        const { success, message, data } = res.data;
        if (!success) {
          showError(message || t('Failed to load pool bindings'));
          return;
        }
        setBindingItems(data?.items || []);
        setBindingTotal(data?.total || 0);
        setBindingPage(data?.page || targetPage);
      } catch (error) {
        showError(getErrorMessage(error, t('Failed to load pool bindings')));
      } finally {
        setBindingLoading(false);
      }
    },
    [bindingPage, bindingTypeFilter, bindingValueFilter, bindingNameFilter, t],
  );

  const loadSubscriptionOrders = useCallback(async (targetPage) => {
    setSubOrderLoading(true);
    try {
      const res = await API.get(
        `/api/pool/subscription_orders?p=${targetPage}&page_size=${PAGE_SIZE}`,
      );
      if (!res?.data?.success) {
        showError(res?.data?.message || t('Failed to load subscription orders'));
        return;
      }
      const data = res.data.data;
      setSubOrderItems(data?.items || []);
      setSubOrderTotal(data?.total || 0);
      setSubOrderPage(data?.page || targetPage);
    } catch (error) {
      showError(getErrorMessage(error, t('Failed to load subscription orders')));
    } finally {
      setSubOrderLoading(false);
    }
  }, [t]);

  const formatTokenSubUnix = (unixSeconds) => {
    if (!unixSeconds) {
      return '—';
    }
    return new Date(unixSeconds * 1000).toLocaleString();
  };

  const loadTokenSubscriptions = useCallback(
    async (targetPage, filters = {}) => {
      setTokenSubLoading(true);
      try {
        const tokenId = filters.tokenId ?? tokenSubFilterTokenId;
        const poolId = filters.poolId ?? tokenSubFilterPoolId;
        const params = new URLSearchParams({
          p: String(targetPage),
          page_size: String(PAGE_SIZE),
        });
        if (String(tokenId).trim()) {
          params.set('token_id', String(tokenId).trim());
        }
        if (String(poolId).trim()) {
          params.set('pool_id', String(poolId).trim());
        }
        const res = await API.get(`/api/pool/token_subscriptions?${params.toString()}`);
        if (!res?.data?.success) {
          showError(res?.data?.message || t('Failed to load token subscriptions'));
          return;
        }
        const data = res.data.data;
        setTokenSubItems(data?.items || []);
        setTokenSubTotal(data?.total || 0);
        setTokenSubPage(data?.page || targetPage);
      } catch (error) {
        showError(getErrorMessage(error, t('Failed to load token subscriptions')));
      } finally {
        setTokenSubLoading(false);
      }
    },
    [tokenSubFilterPoolId, tokenSubFilterTokenId, t],
  );

  const handleTabChange = async (key) => {
    setActiveTab(key);
    if (key === 'pool' && poolItems.length === 0) await loadPools(1);
    if (key === 'channel' && channelItems.length === 0) await loadPoolChannels(1);
    if (key === 'policy' && policyItems.length === 0) await loadPolicies(1);
    if (key === 'binding' && bindingItems.length === 0) await loadBindings(1);
    if (key === 'sub_orders' && subOrderItems.length === 0)
      await loadSubscriptionOrders(1);
    if (key === 'token_subs' && tokenSubItems.length === 0)
      await loadTokenSubscriptions(1);
  };

  const resetPoolForm = () =>
    setPoolForm({
      id: 0,
      name: '',
      description: '',
      status: 1,
      monthly_price_cny: 0,
      monthly_price_cny_input: '0',
      billing_currency: 'CNY',
      billing_period_seconds: 30 * 24 * 3600,
    });
  const resetChannelForm = () =>
    setChannelForm({
      id: 0,
      pool_id: '',
      channel_id: '',
      weight: 0,
      priority: 0,
      enabled: true,
    });
  const resetPolicyForm = () =>
    setPolicyForm({
      id: 0,
      pool_id: '',
      metric: 'request_count',
      scope_type: 'token',
      window_seconds: 5 * 3600,
      limit_count: 1000,
      enabled: true,
    });
  const resetBindingForm = () =>
    setBindingForm({
      id: 0,
      binding_type: 'token',
      binding_value: '',
      pool_id: '',
      priority: 0,
      enabled: true,
    });
  const resetTokenSubForm = () =>
    setTokenSubForm({
      id: 0,
      token_id: '',
      pool_id: '',
      period_end_date: toPeriodEndDateEndOfDay(),
    });
  const openCreateTokenSub = () => {
    resetTokenSubForm();
    setShowTokenSubForm(true);
  };
  const openEditTokenSub = (record) => {
    setTokenSubForm({
      id: record.id,
      token_id: String(record.token_id ?? ''),
      pool_id: String(record.pool_id ?? ''),
      period_end_date: record.period_end
        ? toPeriodEndDateEndOfDay(new Date(record.period_end * 1000))
        : toPeriodEndDateEndOfDay(),
    });
    setShowTokenSubForm(true);
  };
  const closeTokenSubForm = () => {
    setShowTokenSubForm(false);
    resetTokenSubForm();
  };
  const saveTokenSubscription = async () => {
    const tokenId = parseInt(String(tokenSubForm.token_id).trim(), 10);
    const poolId = parseInt(String(tokenSubForm.pool_id).trim(), 10);
    if (!tokenId || !poolId) {
      showError(t('token_id and pool_id are required'));
      return;
    }
    if (!tokenSubForm.period_end_date) {
      showError(t('Subscription period end is required'));
      return;
    }
    const periodEnd = Math.floor(
      toPeriodEndDateEndOfDay(tokenSubForm.period_end_date).getTime() / 1000,
    );
    if (!Number.isFinite(periodEnd) || periodEnd <= 0) {
      showError(t('Invalid subscription period end'));
      return;
    }
    try {
      const res = await API.put('/api/pool/token_subscription', {
        token_id: tokenId,
        pool_id: poolId,
        period_end: periodEnd,
      });
      if (!res?.data?.success) {
        showError(res?.data?.message || t('Failed to save token subscription'));
        return;
      }
      showSuccess(t('Token subscription saved'));
      closeTokenSubForm();
      await loadTokenSubscriptions(tokenSubPage);
    } catch (error) {
      showError(getErrorMessage(error, t('Failed to save token subscription')));
    }
  };
  const revokeTokenSubscriptionNow = async () => {
    const tokenId = parseInt(String(tokenSubForm.token_id).trim(), 10);
    const poolId = parseInt(String(tokenSubForm.pool_id).trim(), 10);
    if (!tokenId || !poolId) {
      return;
    }
    const periodEnd = Math.floor(Date.now() / 1000);
    try {
      const res = await API.put('/api/pool/token_subscription', {
        token_id: tokenId,
        pool_id: poolId,
        period_end: periodEnd,
      });
      if (!res?.data?.success) {
        showError(res?.data?.message || t('Failed to revoke token subscription'));
        return;
      }
      showSuccess(t('Token subscription revoked'));
      closeTokenSubForm();
      await loadTokenSubscriptions(tokenSubPage);
    } catch (error) {
      showError(getErrorMessage(error, t('Failed to revoke token subscription')));
    }
  };
  const openCreatePool = () => {
    resetPoolForm();
    setShowPoolForm(true);
  };
  const openEditPool = (record) => {
    setPoolForm({
      id: record.id,
      name: record.name || '',
      description: record.description || '',
      status: Number(record.status) || 1,
      monthly_price_cny: Number(record.monthly_price_cny) || 0,
      monthly_price_cny_input: formatMonthlyPriceCnyInput(record.monthly_price_cny),
      billing_currency: record.billing_currency || 'CNY',
      billing_period_seconds:
        Number(record.billing_period_seconds) || 30 * 24 * 3600,
    });
    setShowPoolForm(true);
  };
  const closePoolForm = () => {
    setShowPoolForm(false);
    resetPoolForm();
  };
  const openCreateChannel = () => {
    resetChannelForm();
    setShowChannelForm(true);
  };
  const openEditChannel = (record) => {
    setChannelForm({
      id: record.id,
      pool_id: String(record.pool_id || ''),
      channel_id: String(record.channel_id || ''),
      weight: Number(record.weight || 0),
      priority: Number(record.priority || 0),
      enabled: Boolean(record.enabled),
    });
    setShowChannelForm(true);
  };
  const closeChannelForm = () => {
    setShowChannelForm(false);
    resetChannelForm();
  };
  const openCreatePolicy = () => {
    resetPolicyForm();
    setShowPolicyForm(true);
  };
  const openEditPolicy = (record) => {
    setPolicyForm({
      id: record.id,
      pool_id: String(record.pool_id || ''),
      metric: record.metric || 'request_count',
      scope_type: record.scope_type || 'token',
      window_seconds: Number(record.window_seconds || 0),
      limit_count: Number(record.limit_count || 0),
      enabled: Boolean(record.enabled),
    });
    setShowPolicyForm(true);
  };
  const closePolicyForm = () => {
    setShowPolicyForm(false);
    resetPolicyForm();
  };
  const openCreateBinding = () => {
    resetBindingForm();
    setShowBindingForm(true);
  };
  const openEditBinding = (record) => {
    setBindingForm({
      id: record.id,
      binding_type: record.binding_type || 'token',
      binding_value: record.binding_value || '',
      pool_id: String(record.pool_id || ''),
      priority: Number(record.priority || 0),
      enabled: Boolean(record.enabled),
    });
    setShowBindingForm(true);
  };
  const closeBindingForm = () => {
    setShowBindingForm(false);
    resetBindingForm();
  };
  const clearBindingFilters = async () => {
    setBindingTypeFilter('');
    setBindingValueFilter('');
    setBindingNameFilter('');
    setBindingPage(1);
    await loadBindings(1, {
      bindingType: '',
      bindingValue: '',
      bindingName: '',
    });
  };
  const clearChannelFilters = async () => {
    setChannelPoolFilter('');
    setChannelPage(1);
    await loadPoolChannels(1, { poolFilter: '' });
  };
  const clearPolicyFilters = async () => {
    setPolicyPoolFilter('');
    setPolicyPage(1);
    await loadPolicies(1, { poolFilter: '' });
  };

  const savePool = async () => {
    if (!poolForm.name.trim()) {
      showError(t('Pool name is required'));
      return;
    }
    const payload = {
      ...poolForm,
      status: Number(poolForm.status) || 1,
      monthly_price_cny: parseMonthlyPriceCny(poolForm),
      billing_currency: poolForm.billing_currency || 'CNY',
      billing_period_seconds:
        Number(poolForm.billing_period_seconds) || 30 * 24 * 3600,
    };
    try {
      const res =
        poolForm.id > 0
          ? await API.put('/api/pool/', payload)
          : await API.post('/api/pool/', payload);
      if (!res?.data?.success) {
        showError(res?.data?.message || t('Failed to save pool'));
        return;
      }
      showSuccess(t('Saved successfully'));
      closePoolForm();
      await loadPools(poolPage);
    } catch (error) {
      showError(getErrorMessage(error, t('Failed to save pool')));
    }
  };

  const savePoolChannel = async () => {
    if (!channelForm.pool_id || !channelForm.channel_id) {
      showError(t('pool_id and channel_id are required'));
      return;
    }
    const payload = {
      ...channelForm,
      pool_id: Number(channelForm.pool_id),
      channel_id: Number(channelForm.channel_id),
      weight: Number(channelForm.weight) || 0,
      priority: Number(channelForm.priority) || 0,
      enabled: Boolean(channelForm.enabled),
    };
    try {
      const res =
        channelForm.id > 0
          ? await API.put('/api/pool/channel', payload)
          : await API.post('/api/pool/channel', payload);
      if (!res?.data?.success) {
        showError(res?.data?.message || t('Failed to save pool channel'));
        return;
      }
      showSuccess(t('Saved successfully'));
      closeChannelForm();
      await loadPoolChannels(channelPage);
    } catch (error) {
      showError(getErrorMessage(error, t('Failed to save pool channel')));
    }
  };

  const savePolicy = async () => {
    if (!policyForm.pool_id) {
      showError(t('pool_id is required'));
      return;
    }
    if (
      Number(policyForm.window_seconds) <= 0 ||
      Number(policyForm.limit_count) <= 0
    ) {
      showError(t('window_seconds and limit_count must be greater than zero'));
      return;
    }
    const payload = {
      ...policyForm,
      pool_id: Number(policyForm.pool_id),
      window_seconds: Number(policyForm.window_seconds),
      limit_count: Number(policyForm.limit_count),
      enabled: Boolean(policyForm.enabled),
    };
    try {
      const res =
        policyForm.id > 0
          ? await API.put('/api/pool/policy', payload)
          : await API.post('/api/pool/policy', payload);
      if (!res?.data?.success) {
        showError(res?.data?.message || t('Failed to save policy'));
        return;
      }
      showSuccess(t('Saved successfully'));
      closePolicyForm();
      await loadPolicies(policyPage);
    } catch (error) {
      showError(getErrorMessage(error, t('Failed to save policy')));
    }
  };

  const saveBinding = async () => {
    if (
      !bindingForm.pool_id ||
      !bindingForm.binding_type ||
      !bindingForm.binding_value.trim()
    ) {
      showError(t('pool_id, binding_type and binding_value are required'));
      return;
    }
    const payload = {
      ...bindingForm,
      pool_id: Number(bindingForm.pool_id),
      priority: Number(bindingForm.priority) || 0,
      enabled: Boolean(bindingForm.enabled),
      binding_value: bindingForm.binding_value.trim(),
    };
    try {
      const res =
        bindingForm.id > 0
          ? await API.put('/api/pool/binding', payload)
          : await API.post('/api/pool/binding', payload);
      if (!res?.data?.success) {
        showError(res?.data?.message || t('Failed to save binding'));
        return;
      }
      showSuccess(t('Saved successfully'));
      const isCreate = bindingForm.id <= 0;
      if (isCreate) {
        setBindingTypeFilter('');
      }
      closeBindingForm();
      if (isCreate) {
        await loadBindings(1, {
          bindingType: '',
        });
      } else {
        await loadBindings(bindingPage);
      }
    } catch (error) {
      showError(getErrorMessage(error, t('Failed to save binding')));
    }
  };

  const deleteItem = async (endpoint, id, reloadFn, errMsg) => {
    try {
      const res = await API.delete(`${endpoint}/${id}`);
      if (!res?.data?.success) {
        showError(res?.data?.message || errMsg);
        return;
      }
      showSuccess(t('Deleted successfully'));
      await reloadFn();
    } catch (error) {
      showError(getErrorMessage(error, errMsg));
    }
  };
  const confirmDeleteItem = (endpoint, id, reloadFn, errMsg) => {
    Modal.confirm({
      title: t('Confirm delete?'),
      content: t('This operation cannot be undone'),
      onOk: async () => {
        await deleteItem(endpoint, id, reloadFn, errMsg);
      },
    });
  };

  const queryUsage = async () => {
    const poolId = String(usageQuery.pool_id || '').trim();
    const scopeType =
      String(usageQuery.scope_type || 'token').trim().toLowerCase() || 'token';
    const scopeId = String(usageQuery.scope_id || '').trim();
    const windowValue = String(usageQuery.window || '5h').trim() || '5h';

    if (!poolId || !scopeId) {
      const target =
        scopeType === 'token' ? t('token_id') : t('user_id');
      showError(
        t(
          `pool_id and ${target} are required for Query Usage.`,
        ),
      );
      return;
    }
    setUsageLoading(true);
    try {
      const params = new URLSearchParams();
      params.set('pool_id', poolId);
      params.set('scope_type', scopeType);
      params.set('scope_id', scopeId);
      params.set('window', windowValue);
      // Keep explicit legacy keys for both branches to avoid
      // backend-side validation mismatches on mixed deployments.
      params.set('token_id', scopeId);
      params.set('user_id', scopeId);

      const res = await API.get(`/api/pool/usage?${params.toString()}`);
      if (!res?.data?.success) {
        showError(res?.data?.message || t('Failed to query usage'));
        return;
      }
      setUsageResult(res.data.data);
    } catch (error) {
      showError(getErrorMessage(error, t('Failed to query usage')));
    } finally {
      setUsageLoading(false);
    }
  };

  const querySelfUsage = async () => {
    setUsageLoading(true);
    try {
      const res = await API.get(
        `/api/pool/usage/self?window=${encodeURIComponent(usageQuery.window || '5h')}`,
      );
      if (!res?.data?.success) {
        showError(res?.data?.message || t('Failed to query self usage'));
        return;
      }
      setSelfUsageResult(res.data.data);
    } catch (error) {
      showError(getErrorMessage(error, t('Failed to query self usage')));
    } finally {
      setUsageLoading(false);
    }
  };

  const poolColumns = useMemo(
    () => [
      {
        title: 'ID',
        dataIndex: 'id',
        width: 80,
        render: (value) => (
          <Typography.Text copyable={{ content: String(value) }}>
            {value}
          </Typography.Text>
        ),
      },
      { title: 'Name', dataIndex: 'name' },
      { title: 'Description', dataIndex: 'description' },
      {
        title: 'Monthly (CNY)',
        dataIndex: 'monthly_price_cny',
        width: 110,
        render: (v) => (v != null ? String(v) : '0'),
      },
      { title: 'Billing currency', dataIndex: 'billing_currency', width: 110 },
      {
        title: 'Billing period (s)',
        dataIndex: 'billing_period_seconds',
        width: 130,
      },
      {
        title: 'Status',
        dataIndex: 'status',
        render: (value) => (Number(value) === 1 ? 'Enabled' : 'Disabled'),
      },
      {
        title: 'Actions',
        dataIndex: 'operate',
        render: (_, record) => (
          <Space>
            <Button
              size='small'
              type='tertiary'
              onClick={() =>
                openEditPool(record)
              }
            >
              Edit
            </Button>
            <Button
              size='small'
              type='danger'
              onClick={() =>
                confirmDeleteItem(
                  '/api/pool',
                  record.id,
                  () => loadPools(poolPage),
                  t('Failed to delete pool'),
                )
              }
            >
              Delete
            </Button>
          </Space>
        ),
      },
    ],
    [loadPools, poolPage, t],
  );

  const subOrderColumns = useMemo(
    () => [
      { title: 'ID', dataIndex: 'id', width: 72 },
      { title: 'User', dataIndex: 'user_id', width: 72 },
      { title: 'Token', dataIndex: 'token_id', width: 72 },
      { title: 'Pool', dataIndex: 'pool_id', width: 72 },
      {
        title: 'Amount (CNY)',
        dataIndex: 'amount_cny',
        width: 100,
        render: (v) => (v != null ? String(v) : ''),
      },
      { title: 'Currency', dataIndex: 'currency', width: 80 },
      {
        title: 'Fen',
        dataIndex: 'amount_total_fen',
        width: 80,
      },
      { title: 'Status', dataIndex: 'status', width: 100 },
      {
        title: 'Trade no',
        dataIndex: 'trade_no',
        width: 160,
        render: (value) => (
          <Typography.Text copyable={{ content: String(value || '') }}>
            {value}
          </Typography.Text>
        ),
      },
      {
        title: 'WeChat txn',
        dataIndex: 'wechat_transaction_id',
        width: 140,
        render: (value) => (
          <Typography.Text copyable={{ content: String(value || '') }}>
            {value || '-'}
          </Typography.Text>
        ),
      },
      { title: 'Created', dataIndex: 'create_time', width: 110 },
      { title: 'Completed', dataIndex: 'complete_time', width: 110 },
    ],
    [],
  );

  const tokenSubColumns = useMemo(
    () => [
      { title: 'ID', dataIndex: 'id', width: 72 },
      { title: 'Token', dataIndex: 'token_id', width: 72 },
      { title: 'Pool', dataIndex: 'pool_id', width: 72 },
      {
        title: t('Period start'),
        dataIndex: 'period_start',
        width: 168,
        render: (v) => formatTokenSubUnix(v),
      },
      {
        title: t('Period end'),
        dataIndex: 'period_end',
        width: 168,
        render: (v) => formatTokenSubUnix(v),
      },
      {
        title: t('Active'),
        dataIndex: 'active',
        width: 88,
        render: (v) => boolTag(v),
      },
      { title: t('Last order'), dataIndex: 'last_order_id', width: 96 },
      {
        title: 'Actions',
        dataIndex: 'operate',
        render: (_, record) => (
          <Button size='small' type='tertiary' onClick={() => openEditTokenSub(record)}>
            {t('Edit')}
          </Button>
        ),
      },
    ],
    [t],
  );

  const channelColumns = useMemo(
    () => [
      { title: 'ID', dataIndex: 'id', width: 80 },
      { title: 'Pool ID', dataIndex: 'pool_id', width: 88 },
      {
        title: 'Pool name',
        dataIndex: 'pool_name',
        ellipsis: true,
        render: (v) => v || '—',
      },
      { title: 'Channel ID', dataIndex: 'channel_id', width: 100 },
      {
        title: 'Channel name',
        dataIndex: 'channel_name',
        ellipsis: true,
        render: (v) => v || '—',
      },
      { title: 'Weight', dataIndex: 'weight', width: 88 },
      { title: 'Priority', dataIndex: 'priority', width: 96 },
      { title: 'Enabled', dataIndex: 'enabled', width: 96, render: boolTag },
      {
        title: 'Actions',
        dataIndex: 'operate',
        render: (_, record) => (
          <Space>
            <Button
              size='small'
              type='tertiary'
              onClick={() =>
                openEditChannel(record)
              }
            >
              Edit
            </Button>
            <Button
              size='small'
              type='danger'
              onClick={() =>
                confirmDeleteItem(
                  '/api/pool/channel',
                  record.id,
                  () => loadPoolChannels(channelPage),
                  t('Failed to delete pool channel'),
                )
              }
            >
              Delete
            </Button>
          </Space>
        ),
      },
    ],
    [channelPage, loadPoolChannels, t],
  );

  const policyColumns = useMemo(
    () => [
      { title: 'ID', dataIndex: 'id', width: 80 },
      { title: 'Pool ID', dataIndex: 'pool_id', width: 88 },
      {
        title: 'Pool name',
        dataIndex: 'pool_name',
        ellipsis: true,
        render: (v) => v || '—',
      },
      { title: 'Metric', dataIndex: 'metric', width: 120 },
      { title: 'Scope', dataIndex: 'scope_type', width: 88 },
      { title: 'Window(s)', dataIndex: 'window_seconds', width: 110 },
      { title: 'Limit', dataIndex: 'limit_count', width: 88 },
      { title: 'Enabled', dataIndex: 'enabled', width: 96, render: boolTag },
      {
        title: 'Actions',
        dataIndex: 'operate',
        render: (_, record) => (
          <Space>
            <Button
              size='small'
              type='tertiary'
              onClick={() =>
                openEditPolicy(record)
              }
            >
              Edit
            </Button>
            <Button
              size='small'
              type='danger'
              onClick={() =>
                confirmDeleteItem(
                  '/api/pool/policy',
                  record.id,
                  () => loadPolicies(policyPage),
                  t('Failed to delete policy'),
                )
              }
            >
              Delete
            </Button>
          </Space>
        ),
      },
    ],
    [loadPolicies, policyPage, t],
  );

  const bindingColumns = useMemo(
    () => [
      { title: 'ID', dataIndex: 'id', width: 80 },
      { title: 'Binding Type', dataIndex: 'binding_type' },
      { title: 'Binding Value', dataIndex: 'binding_value' },
      { title: 'Binding Name', dataIndex: 'binding_name' },
      { title: 'Pool ID', dataIndex: 'pool_id' },
      { title: 'Pool Name', dataIndex: 'pool_name' },
      { title: 'Priority', dataIndex: 'priority' },
      { title: 'Enabled', dataIndex: 'enabled', render: boolTag },
      {
        title: 'Actions',
        dataIndex: 'operate',
        render: (_, record) => (
          <Space>
            <Button
              size='small'
              type='tertiary'
              onClick={() =>
                openEditBinding(record)
              }
            >
              Edit
            </Button>
            <Button
              size='small'
              type='danger'
              onClick={() =>
                confirmDeleteItem(
                  '/api/pool/binding',
                  record.id,
                  () => loadBindings(bindingPage),
                  t('Failed to delete binding'),
                )
              }
            >
              Delete
            </Button>
          </Space>
        ),
      },
    ],
    [bindingPage, loadBindings, t],
  );

  useEffect(() => {
    loadBindings(1);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return {
    PAGE_SIZE,
    t,
    activeTab,
    setActiveTab,
    handleTabChange,
    loadPools,
    loadPoolChannels,
    loadPolicies,
    loadBindings,

    poolItems,
    poolTotal,
    poolPage,
    poolLoading,
    poolForm,
    setPoolForm,
    showPoolForm,
    openCreatePool,
    closePoolForm,
    poolColumns,
    resetPoolForm,
    savePool,

    channelItems,
    channelTotal,
    channelPage,
    channelLoading,
    channelForm,
    setChannelForm,
    showChannelForm,
    openCreateChannel,
    closeChannelForm,
    channelColumns,
    channelPoolFilter,
    setChannelPoolFilter,
    clearChannelFilters,
    resetChannelForm,
    savePoolChannel,

    policyItems,
    policyTotal,
    policyPage,
    policyLoading,
    policyForm,
    setPolicyForm,
    showPolicyForm,
    openCreatePolicy,
    closePolicyForm,
    policyColumns,
    policyPoolFilter,
    setPolicyPoolFilter,
    clearPolicyFilters,
    resetPolicyForm,
    savePolicy,

    bindingItems,
    bindingTotal,
    bindingPage,
    bindingLoading,
    bindingForm,
    setBindingForm,
    showBindingForm,
    openCreateBinding,
    closeBindingForm,
    bindingColumns,
    bindingTypeFilter,
    setBindingTypeFilter,
    bindingValueFilter,
    setBindingValueFilter,
    bindingNameFilter,
    setBindingNameFilter,
    clearBindingFilters,
    resetBindingForm,
    saveBinding,

    subOrderItems,
    subOrderTotal,
    subOrderPage,
    subOrderLoading,
    loadSubscriptionOrders,
    subOrderColumns,

    tokenSubItems,
    tokenSubTotal,
    tokenSubPage,
    tokenSubLoading,
    tokenSubFilterTokenId,
    setTokenSubFilterTokenId,
    tokenSubFilterPoolId,
    setTokenSubFilterPoolId,
    loadTokenSubscriptions,
    tokenSubColumns,
    tokenSubForm,
    setTokenSubForm,
    showTokenSubForm,
    openCreateTokenSub,
    closeTokenSubForm,
    saveTokenSubscription,
    revokeTokenSubscriptionNow,

    usageLoading,
    usageQuery,
    setUsageQuery,
    usageResult,
    selfUsageResult,
    queryUsage,
    querySelfUsage,
  };
};

