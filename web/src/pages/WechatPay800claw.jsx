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

import React, { useEffect, useState, useCallback, useRef } from 'react';
import { useSearchParams } from 'react-router-dom';

const API_BASE = '';
const ERUDA_CDN = 'https://cdn.bootcdn.net/ajax/libs/eruda/3.4.1/eruda.min.js';
const IS_DEV = import.meta.env.DEV;

const MOCK_CHECKOUT = {
  prepay_id: 'wx190317106985061a0a0e85510085fc0000',
  jsapi_pay_params: {
    appId: 'wxc9ce483d04c0563d',
    timeStamp: '1784402230',
    nonceStr: '94HMxxxxHSHW',
    package: 'prepay_id=wx190317106985061a0a0e85510085fc0000',
    signType: 'RSA',
    paySign: 'mock_pay_sign_xxxxxxxx',
  },
  wx_config: {
    appId: 'wxc9ce483d04c0563d',
    timestamp: 1784402230,
    nonceStr: '7VXPxxxxOU1k',
    signature: 'mock_signature_xxxx',
  },
  pool_name: 'Lite',
  amount_fen: 4000,
  base_amount_fen: 5000,
  period_months: 1,
  trade_no: 'TP4ogcxxxxxxxxx',
  status: 'pending',
};

const C = {
  brand: '#FF6A00',
  green: '#07C160',
  text: '#1F2937',
  text2: '#6B7280',
  text3: '#9CA3AF',
  bg: '#F5F5F5',
  card: '#FFFFFF',
  border: '#E5E7EB',
};

function formatFen(fen) {
  if (fen == null) return '';
  return (fen / 100).toFixed(2);
}

function periodLabel(months) {
  if (months === 1) return '1 个月';
  return `${months} 个月`;
}

function mask(str, keep = 4) {
  if (!str || typeof str !== 'string' || str.length <= keep * 2) return str;
  return str.slice(0, keep) + '***' + str.slice(-keep);
}

function nowTime() {
  return new Date().toLocaleTimeString('zh-CN', { hour12: false });
}

function BrandHeader() {
  return (
    <div className="flex items-center justify-center gap-2 mb-12">
      <img src="/logo-800claw.png" alt="800claw" style={{ width: 24, height: 24, borderRadius: '50%', boxShadow: '0 2px 6px rgba(0,0,0,0.08)' }} />
      <span style={{ color: C.text3, fontSize: '14px' }}>养虾匠</span>
    </div>
  );
}

function CardShell({ children }) {
  return (
    <div className="flex items-center justify-center min-h-screen p-4" style={{ background: C.bg }}>
      <div className="w-full max-w-sm relative" style={{ background: C.card, borderRadius: '16px', boxShadow: '0 4px 24px rgba(0,0,0,0.06)', padding: '32px 24px 48px 24px' }}>
        <div className="text-center">
          <BrandHeader />
          {children}
        </div>
      </div>
    </div>
  );
}

function DebugPanel({ logs, open, onToggle }) {
  const copyLogs = useCallback(() => {
    const text = logs.map((l) => `[${l.time}][${l.tag}] ${l.data}`).join('\n');
    if (navigator.clipboard) {
      navigator.clipboard.writeText(text).catch(() => {});
    } else {
      const ta = document.createElement('textarea');
      ta.value = text;
      document.body.appendChild(ta);
      ta.select();
      document.execCommand('copy');
      document.body.removeChild(ta);
    }
  }, [logs]);

  return (
    <div style={{ position: 'fixed', bottom: '12px', right: '12px', zIndex: 9999 }}>
      {!open ? (
        <button
          onClick={onToggle}
          title="调试日志"
          style={{ width: 40, height: 40, borderRadius: '50%', background: '#333', color: '#fff', border: 'none', fontSize: 20, cursor: 'pointer', opacity: 0.75, display: 'flex', alignItems: 'center', justifyContent: 'center' }}
        >
          🐛
        </button>
      ) : (
        <div style={{ width: 320, maxHeight: 420, background: 'rgba(0,0,0,0.88)', borderRadius: 12, overflow: 'hidden', display: 'flex', flexDirection: 'column', backdropFilter: 'blur(4px)' }}>
          <div style={{ padding: '8px 12px', borderBottom: '1px solid #444', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span style={{ color: '#22c55e', fontSize: 12, fontWeight: 600 }}>调试日志 ({logs.length})</span>
            <div style={{ display: 'flex', gap: 8 }}>
              <button
                onClick={copyLogs}
                style={{ background: 'none', border: '1px solid #22c55e', color: '#22c55e', borderRadius: 4, fontSize: 11, padding: '2px 8px', cursor: 'pointer' }}
              >
                复制
              </button>
              <button onClick={onToggle} style={{ background: 'none', border: 'none', color: '#fff', fontSize: 16, cursor: 'pointer' }}>
                ✕
              </button>
            </div>
          </div>
          <div style={{ padding: 8, overflowY: 'auto', maxHeight: 360, fontFamily: 'monospace', fontSize: 11, lineHeight: 1.5 }}>
            {logs.length === 0 ? (
              <div style={{ color: '#666', textAlign: 'center', padding: '20px 0' }}>暂无日志</div>
            ) : (
              logs.map((log, i) => (
                <div key={i} style={{ marginBottom: 6 }}>
                  <div style={{ color: '#888' }}>
                    [{log.time}][{log.tag}]
                  </div>
                  <pre style={{ margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-all', color: '#4ade80' }}>{log.data}</pre>
                </div>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function DevStateSwitcher({ current, onChange }) {
  const states = ['loading', 'ready', 'error', 'failed', 'cancelled', 'verifying', 'timeout', 'success'];
  return (
    <div style={{ position: 'fixed', top: 12, left: 12, zIndex: 9999, background: 'rgba(0,0,0,0.85)', borderRadius: 8, padding: '8px 10px', backdropFilter: 'blur(4px)' }}>
      <div style={{ color: '#aaa', fontSize: 10, marginBottom: 6, fontWeight: 600 }}>DEV MOCK</div>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
        {states.map((s) => (
          <button
            key={s}
            onClick={() => onChange(s)}
            style={{
              border: 'none',
              borderRadius: 4,
              padding: '3px 8px',
              fontSize: 11,
              cursor: 'pointer',
              background: current === s ? '#22c55e' : '#444',
              color: '#fff',
            }}
          >
            {s}
          </button>
        ))}
      </div>
    </div>
  );
}

export default function WechatPay800claw() {
  const [searchParams] = useSearchParams();
  const [state, setState] = useState('loading');
  const [error, setError] = useState('');
  const [checkout, setCheckout] = useState(null);
  const pollIntervalRef = useRef(null);
  const pollTimeoutRef = useRef(null);

  const isDebug = useRef(new URLSearchParams(window.location.search).get('debug') === '1').current;
  const [panelOpen, setPanelOpen] = useState(false);
  const logsRef = useRef([]);
  const [logTick, setLogTick] = useState(0);

  const addLog = useCallback(
    (tag, data) => {
      const entry = {
        time: nowTime(),
        tag,
        data: typeof data === 'object' ? JSON.stringify(data, null, 2) : String(data),
      };
      logsRef.current = [...logsRef.current, entry].slice(-100);
      setLogTick((t) => t + 1);
      if (isDebug) {
        // eslint-disable-next-line no-console
        console.log(`[${entry.time}][${tag}]`, data);
      }
    },
    [isDebug]
  );

  // Load eruda from China CDN when debug=1
  useEffect(() => {
    if (isDebug && !window.eruda) {
      const script = document.createElement('script');
      script.src = ERUDA_CDN;
      script.crossOrigin = 'anonymous';
      script.onload = () => {
        if (window.eruda) window.eruda.init();
        addLog('eruda', 'eruda loaded from BootCDN');
      };
      script.onerror = () => {
        addLog('eruda', 'eruda load failed');
      };
      document.body.appendChild(script);
    }
  }, [isDebug, addLog]);

  useEffect(() => {
    document.title = '养虾匠';
  }, []);

  const code = searchParams.get('code') || '';
  const stateParam = searchParams.get('state') || '';

  useEffect(() => {
    addLog('mount', {
      code_present: !!code,
      code_len: code?.length,
      state_present: !!stateParam,
      state_len: stateParam?.length,
      debug: isDebug,
      dev: IS_DEV,
      ua: navigator.userAgent?.slice(0, 120),
      href: window.location.href.split('?')[0],
    });
  }, [addLog, code, stateParam, isDebug]);

  const initPayment = useCallback(async () => {
    // ====== dev mock preview mode ======
    const urlParams = new URLSearchParams(window.location.search);
    const mock = urlParams.get('mock');
    if (IS_DEV && mock) {
      addLog('mock', { target: mock });
      if (mock === 'ready' || mock === 'success' || mock === 'cancelled' || mock === 'failed') {
        setCheckout(MOCK_CHECKOUT);
      }
      if (mock === 'error' || mock === 'failed') {
        setError('支付验证签名失败');
      }
      setState(mock);
      return;
    }
    // ====== real api flow ======
    if (!code || !stateParam) {
      setError('缺少支付参数');
      setState('error');
      return;
    }

    try {
      const pageUrl = window.location.href.split('#')[0];
      addLog('checkout-req', {
        state: mask(stateParam),
        code: mask(code),
        page_url: pageUrl,
      });

      const resp = await fetch(`${API_BASE}/api/usage/token/pool/subscription/wechat/jsapi/checkout`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Accept: 'application/json',
        },
        body: JSON.stringify({
          state: stateParam,
          code,
          page_url: pageUrl,
        }),
      });

      const raw = await resp.json();
      const data = raw.data || raw;

      addLog('checkout-res', {
        status: resp.status,
        ok: resp.ok,
        prepay_id: data.prepay_id ? mask(data.prepay_id, 8) : null,
        pool_name: data.pool_name,
        amount_fen: data.amount_fen,
        period_months: data.period_months,
        wx_config_appId: data.wx_config?.appId,
        wx_config_timestamp: data.wx_config?.timestamp,
        wx_config_nonceStr: data.wx_config?.nonceStr ? mask(data.wx_config.nonceStr) : null,
        wx_config_signature: data.wx_config?.signature ? '***' : null,
        jsapi_timestamp: data.jsapi_pay_params?.timeStamp,
        jsapi_nonceStr: data.jsapi_pay_params?.nonceStr ? mask(data.jsapi_pay_params.nonceStr) : null,
        jsapi_signType: data.jsapi_pay_params?.signType,
        jsapi_paySign: data.jsapi_pay_params?.paySign ? '***' : null,
      });

      if (!resp.ok || !data.prepay_id) {
        setError(data.message || '创建订单失败');
        setState('error');
        return;
      }

      setCheckout(data);
      setState('ready');
    } catch (e) {
      addLog('checkout-err', { message: e?.message, stack: e?.stack?.slice(0, 200) });
      setError('网络错误：' + (e.message || ''));
      setState('error');
    }
  }, [code, stateParam, addLog]);

  useEffect(() => {
    initPayment();
  }, [initPayment]);

  const pollOrderStatus = useCallback(async (tradeNo) => {
    try {
      const resp = await fetch(`${API_BASE}/api/usage/token/pool/subscription/wechat/jsapi/order?trade_no=${encodeURIComponent(tradeNo)}`);
      const raw = await resp.json();
      const data = raw.data || raw;
      addLog('poll-order', { status: data.status, reconciled: data.reconciled_from_wechat });

      if (data.status === 'success') {
        clearInterval(pollIntervalRef.current);
        clearTimeout(pollTimeoutRef.current);
        setState('success');
        return true;
      }
      if (data.status === 'expired' || data.status === 'failed') {
        clearInterval(pollIntervalRef.current);
        clearTimeout(pollTimeoutRef.current);
        setError('订单已过期或支付失败');
        setState('failed');
        return true;
      }
      return false;
    } catch (e) {
      addLog('poll-err', e?.message);
      return false;
    }
  }, [addLog]);

  const startPolling = useCallback((tradeNo) => {
    setState('verifying');
    addLog('poll-start', { trade_no: tradeNo });

    // immediate first check
    pollOrderStatus(tradeNo);

    pollIntervalRef.current = setInterval(() => {
      pollOrderStatus(tradeNo);
    }, 2500);

    pollTimeoutRef.current = setTimeout(() => {
      clearInterval(pollIntervalRef.current);
      addLog('poll-timeout', 'polling timed out');
      setError('支付状态确认超时，请稍后刷新页面查看');
      setState('timeout');
    }, 60000);
  }, [addLog, pollOrderStatus]);

  useEffect(() => {
    return () => {
      clearInterval(pollIntervalRef.current);
      clearTimeout(pollTimeoutRef.current);
    };
  }, []);

  const handlePay = useCallback(() => {
    if (!checkout || !window.wx) {
      addLog('pay-check', { checkout: !!checkout, wx_sdk: !!window.wx });
      setError('微信 SDK 未加载');
      return;
    }

    const { jsapi_pay_params, wx_config } = checkout;

    addLog('wx.config', {
      appId: wx_config.appId,
      timestamp: wx_config.timestamp,
      nonceStr: mask(wx_config.nonceStr),
      signature: '***',
      jsApiList: ['chooseWXPay'],
    });

    window.wx.config({
      debug: false,
      appId: wx_config.appId,
      timestamp: wx_config.timestamp,
      nonceStr: wx_config.nonceStr,
      signature: wx_config.signature,
      jsApiList: ['chooseWXPay'],
    });

    window.wx.ready(() => {
      addLog('wx.config-ok', 'wx.config ready');

      const payPayload = {
        appId: jsapi_pay_params.appId,
        timestamp: String(jsapi_pay_params.timeStamp),
        nonceStr: mask(jsapi_pay_params.nonceStr),
        package: jsapi_pay_params.package,
        signType: jsapi_pay_params.signType,
        paySign: '***',
      };
      addLog('chooseWXPay', payPayload);

      window.wx.chooseWXPay({
        appId: jsapi_pay_params.appId,
        timestamp: String(jsapi_pay_params.timeStamp),
        nonceStr: jsapi_pay_params.nonceStr,
        package: jsapi_pay_params.package,
        signType: jsapi_pay_params.signType,
        paySign: jsapi_pay_params.paySign,
        success: () => {
          addLog('pay-cb', 'success');
          if (checkout?.trade_no) {
            startPolling(checkout.trade_no);
          } else {
            setState('success');
          }
        },
        cancel: () => {
          addLog('pay-cb', 'cancel');
          setState('cancelled');
        },
        fail: (err) => {
          addLog('pay-cb', { fail: err?.errMsg, err });
          setError(err?.errMsg || '支付失败');
          setState('failed');
        },
      });
    });

    window.wx.error((err) => {
      addLog('wx.config-fail', { errMsg: err?.errMsg, err });
      setError(err?.errMsg || '微信 SDK 初始化失败');
      setState('error');
    });
  }, [checkout, addLog, startPolling]);

  const closePage = useCallback(() => {
    if (window.WeixinJSBridge) window.WeixinJSBridge.call('closeWindow');
  }, []);

  useEffect(() => {
    addLog('state-change', { state, error: error || null });
  }, [state, error, addLog]);

  // dev: allow state switch without reload
  const handleMockStateChange = useCallback((next) => {
    if (next === 'ready' || next === 'success' || next === 'cancelled' || next === 'failed' || next === 'verifying' || next === 'timeout') {
      setCheckout(MOCK_CHECKOUT);
    } else {
      setCheckout(null);
    }
    if (next === 'error' || next === 'failed' || next === 'timeout') {
      setError(next === 'timeout' ? '支付状态确认超时，请稍后刷新页面查看' : '支付验证签名失败');
    } else {
      setError('');
    }
    setState(next);
    addLog('mock-switch', { to: next });
  }, [addLog]);

  if (state === 'loading') {
    return (
      <CardShell>
        <div style={{ color: C.text3, fontSize: '12px', marginBottom: 20 }}>正在准备支付...</div>
        {IS_DEV && <DevStateSwitcher current={state} onChange={handleMockStateChange} />}
        {isDebug && <DebugPanel logs={logsRef.current} open={panelOpen} onToggle={() => setPanelOpen((v) => !v)} />}
      </CardShell>
    );
  }

  if (state === 'verifying') {
    return (
      <CardShell>
        <div style={{ color: C.green, fontSize: '20px', fontWeight: 700, marginTop: 16 }}>支付成功</div>
        <div className="mb-10" style={{ color: C.text2, fontSize: '14px', marginTop: 8 }}>正在确认订单状态，请稍候...</div>
        <div style={{ color: C.text3, fontSize: '13px' }}>
          {checkout?.pool_name || ''} / {periodLabel(checkout?.period_months)}
        </div>
        <div style={{ color: C.brand, fontSize: '28px', fontWeight: 600, marginTop: 12 }}>
          <span style={{ fontSize: '14px' }}>¥</span>{formatFen(checkout?.amount_fen)}
        </div>
        {IS_DEV && <DevStateSwitcher current={state} onChange={handleMockStateChange} />}
        {isDebug && <DebugPanel logs={logsRef.current} open={panelOpen} onToggle={() => setPanelOpen((v) => !v)} />}
      </CardShell>
    );
  }

  if (state === 'timeout') {
    return (
      <CardShell>
        <div className="mb-2" style={{ color: C.text, fontSize: '18px', fontWeight: 600, marginTop: 16 }}>状态确认超时</div>
        <div className="mb-10" style={{ color: C.text2, fontSize: '14px' }}>{error || '支付状态确认超时，请稍后刷新页面查看'}</div>
        <button
          onClick={closePage}
          className="w-full rounded-xl py-3 text-base font-semibold"
          style={{ background: C.green, color: '#fff', border: 'none', height: '48px', cursor: 'pointer' }}
        >
          完成
        </button>
        {IS_DEV && <DevStateSwitcher current={state} onChange={handleMockStateChange} />}
        {isDebug && <DebugPanel logs={logsRef.current} open={panelOpen} onToggle={() => setPanelOpen((v) => !v)} />}
      </CardShell>
    );
  }

  if (state === 'error' || state === 'failed') {
    return (
      <CardShell>
        <div className="mb-2" style={{ color: C.text, fontSize: '18px', fontWeight: 600, marginTop: 16 }}>支付未完成</div>
        <div className="mb-10" style={{ color: C.text2, fontSize: '14px' }}>{error || '支付失败，请重试'}</div>
        {state === 'failed' ? (
          <button
            onClick={handlePay}
            className="w-full rounded-xl py-3 text-base font-semibold mb-3"
            style={{ background: C.green, color: '#fff', border: 'none', height: '48px', cursor: 'pointer' }}
          >
            重新支付
          </button>
        ) : null}
        <button
          onClick={closePage}
          className="w-full rounded-xl py-3 text-sm font-semibold"
          style={{ background: '#FFFFFF', color: C.text2, border: `1px solid ${C.border}`, height: '48px', cursor: 'pointer' }}
        >
          关闭页面
        </button>
        {IS_DEV && <DevStateSwitcher current={state} onChange={handleMockStateChange} />}
        {isDebug && <DebugPanel logs={logsRef.current} open={panelOpen} onToggle={() => setPanelOpen((v) => !v)} />}
      </CardShell>
    );
  }

  if (state === 'cancelled') {
    return (
      <CardShell>
        <div className="mb-2" style={{ color: C.text, fontSize: '18px', fontWeight: 600, marginTop: 16 }}>支付已取消</div>
        <div className="mb-10" style={{ color: C.text2, fontSize: '14px' }}>你取消了本次支付</div>
        <button
          onClick={handlePay}
          className="w-full rounded-xl py-3 text-base font-semibold mb-3"
          style={{ background: C.green, color: '#fff', border: 'none', height: '48px', cursor: 'pointer' }}
        >
          重新支付
        </button>
        <button
          onClick={closePage}
          className="w-full rounded-xl py-3 text-sm font-semibold"
          style={{ background: '#FFFFFF', color: C.text2, border: `1px solid ${C.border}`, height: '48px', cursor: 'pointer' }}
        >
          关闭页面
        </button>
        {IS_DEV && <DevStateSwitcher current={state} onChange={handleMockStateChange} />}
        {isDebug && <DebugPanel logs={logsRef.current} open={panelOpen} onToggle={() => setPanelOpen((v) => !v)} />}
      </CardShell>
    );
  }

  if (state === 'success') {
    return (
      <CardShell>
        <div className="mb-3" style={{ color: C.green, fontSize: '20px', fontWeight: 700, marginTop: 16 }}>支付成功</div>
        <div className="mb-0" style={{ color: C.text2, fontSize: '14px' }}>
          {checkout?.pool_name} / {periodLabel(checkout?.period_months)}
        </div>
        <div className="mb-10" style={{ color: C.text2, fontSize: '14px' }}>已成功续期</div>
        <button
          onClick={closePage}
          className="w-full rounded-xl py-3 text-base font-semibold"
          style={{ background: C.green, color: '#fff', border: 'none', height: '48px', cursor: 'pointer' }}
        >
          完成
        </button>
        {IS_DEV && <DevStateSwitcher current={state} onChange={handleMockStateChange} />}
        {isDebug && <DebugPanel logs={logsRef.current} open={panelOpen} onToggle={() => setPanelOpen((v) => !v)} />}
      </CardShell>
    );
  }

  const hasDiscount = checkout && checkout.base_amount_fen > checkout.amount_fen;
  const savings = hasDiscount ? checkout.base_amount_fen - checkout.amount_fen : 0;

  return (
    <div className="flex items-center justify-center min-h-screen p-4" style={{ background: C.bg }}>
      <div className="w-full max-w-sm relative" style={{ background: C.card, borderRadius: '16px', boxShadow: '0 4px 24px rgba(0,0,0,0.06)', padding: '32px 24px' }}>
        <div className="text-center">
          <BrandHeader />

          <img src="/wxpay-logo.png" alt="微信支付" style={{ width: 120, height: 'auto', margin: '10px auto 12px', display: 'block' }} />

          <div className="mb-6" style={{ color: C.brand, fontSize: '48px', fontWeight: 700, lineHeight: 1.2 }}>
            <span style={{ fontSize: '20px', position: 'relative', bottom: 2, marginRight: 4, fontWeight: 600 }}>¥</span>
            {formatFen(checkout?.amount_fen)}
          </div>

          <div className="mb-1" style={{ color: C.text, fontSize: '15px' }}>
            {checkout?.pool_name || ''} / {periodLabel(checkout?.period_months)}
          </div>

          {hasDiscount ? (
            <div className="mb-1" style={{ color: C.text3, fontSize: '10px' }}>
              <span style={{ textDecoration: 'line-through' }}>原价 ¥{formatFen(checkout?.base_amount_fen)}</span>
              {' / '}
              <span style={{ color: C.green }}>省 ¥{formatFen(savings)}</span>
            </div>
          ) : null}

          <div className="mt-2 mb-8" style={{ color: C.text3, fontSize: '13px' }}>
            支付方式：微信支付
          </div>

          <button
            onClick={handlePay}
            className="w-full rounded-xl py-3 text-base font-semibold mb-4"
            style={{ background: C.green, color: '#fff', border: 'none', height: '48px', cursor: 'pointer' }}
          >
            确认支付
          </button>

          <div className="inline-flex items-center px-2 py-0.5 rounded-full" style={{ background: '#F0FDF4', color: C.green, fontSize: '11px', fontWeight: 500 }}>
            安全支付
          </div>
        </div>
      </div>
      {IS_DEV && <DevStateSwitcher current={state} onChange={handleMockStateChange} />}
      {isDebug && <DebugPanel logs={logsRef.current} open={panelOpen} onToggle={() => setPanelOpen((v) => !v)} />}
    </div>
  );
}
