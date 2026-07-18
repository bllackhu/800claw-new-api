import React, { useEffect, useState, useCallback } from 'react';
import { useSearchParams } from 'react-router-dom';

const API_BASE = '';

function formatFen(fen) {
  if (fen == null) return '';
  return (fen / 100).toFixed(2);
}

function periodLabel(months) {
  if (months === 1) return '1 个月';
  return `${months} 个月`;
}

export default function WechatPay800claw() {
  const [searchParams] = useSearchParams();
  const [state, setState] = useState('loading');
  const [error, setError] = useState('');
  const [checkout, setCheckout] = useState(null);

  const code = searchParams.get('code') || '';
  const stateParam = searchParams.get('state') || '';

  const poolId = parseInt(stateParam.split(':')[0] || '0', 10);
  const periodMonths = parseInt(stateParam.split(':')[1] || '1', 10);

  const initPayment = useCallback(async () => {
    if (!code || !poolId) {
      setError('缺少支付参数');
      setState('error');
      return;
    }

    try {
      const pageUrl = window.location.href.split('#')[0];
      const resp = await fetch(`${API_BASE}/api/usage/token/pool/subscription/wechat/jsapi/checkout`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Accept: 'application/json',
        },
        body: JSON.stringify({
          pool_id: poolId,
          period_months: periodMonths,
          code,
          page_url: pageUrl,
        }),
      });

      const raw = await resp.json();
      const data = raw.data || raw;

      if (!resp.ok || !data.prepay_id) {
        setError(data.message || '创建订单失败');
        setState('error');
        return;
      }

      setCheckout(data);
      setState('ready');
    } catch (e) {
      setError('网络错误：' + (e.message || ''));
      setState('error');
    }
  }, [code, poolId, periodMonths]);

  useEffect(() => {
    initPayment();
  }, [initPayment]);

  const handlePay = useCallback(() => {
    if (!checkout || !window.wx) {
      setError('微信 SDK 未加载');
      return;
    }

    const { jsapi_pay_params, wx_config } = checkout;

    window.wx.config({
      debug: false,
      appId: wx_config.appId,
      timestamp: wx_config.timestamp,
      nonceStr: wx_config.nonceStr,
      signature: wx_config.signature,
      jsApiList: ['chooseWXPay'],
    });

    window.wx.ready(() => {
      window.wx.chooseWXPay({
        appId: jsapi_pay_params.appId,
        timestamp: jsapi_pay_params.timeStamp,
        nonceStr: jsapi_pay_params.nonceStr,
        package: jsapi_pay_params.package,
        signType: jsapi_pay_params.signType,
        paySign: jsapi_pay_params.paySign,
        success: () => {
          setState('success');
        },
        cancel: () => {
          setState('cancelled');
        },
        fail: (err) => {
          setError(err?.errMsg || '支付失败');
          setState('failed');
        },
      });
    });

    window.wx.error((err) => {
      setError(err?.errMsg || '微信 SDK 初始化失败');
      setState('error');
    });
  }, [checkout]);

  if (state === 'loading') {
    return (
      <div className="flex items-center justify-center min-h-screen" style={{ background: 'var(--semi-color-bg-0)' }}>
        <div className="text-center">
          <div className="mb-4" style={{ color: 'var(--semi-color-text-2)', fontSize: '14px' }}>正在准备支付...</div>
        </div>
      </div>
    );
  }

  if (state === 'error' || state === 'failed') {
    return (
      <div className="flex items-center justify-center min-h-screen p-4" style={{ background: 'var(--semi-color-bg-0)' }}>
        <div className="text-center max-w-sm w-full">
          <div className="mb-4" style={{ fontSize: '48px' }}>❌</div>
          <div className="mb-2" style={{ color: 'var(--semi-color-text-0)', fontSize: '18px', fontWeight: 600 }}>支付未完成</div>
          <div className="mb-6" style={{ color: 'var(--semi-color-text-2)', fontSize: '14px' }}>{error || '支付失败，请重试'}</div>
          {state === 'failed' ? (
            <button
              onClick={handlePay}
              className="w-full rounded-xl py-3 text-sm font-semibold mb-3"
              style={{ background: 'var(--semi-color-primary)', color: '#fff', border: 'none', height: '48px' }}
            >
              重新支付
            </button>
          ) : null}
          <button
            onClick={() => { if (window.WeixinJSBridge) window.WeixinJSBridge.call('closeWindow'); }}
            className="w-full rounded-xl py-3 text-sm font-semibold"
            style={{ background: 'var(--semi-color-fill-1)', color: 'var(--semi-color-text-1)', border: `1px solid var(--semi-color-border)`, height: '48px' }}
          >
            关闭页面
          </button>
        </div>
      </div>
    );
  }

  if (state === 'cancelled') {
    return (
      <div className="flex items-center justify-center min-h-screen p-4" style={{ background: 'var(--semi-color-bg-0)' }}>
        <div className="text-center max-w-sm w-full">
          <div className="mb-4" style={{ fontSize: '48px' }}>🛡️</div>
          <div className="mb-2" style={{ color: 'var(--semi-color-text-0)', fontSize: '18px', fontWeight: 600 }}>支付已取消</div>
          <div className="mb-6" style={{ color: 'var(--semi-color-text-2)', fontSize: '14px' }}>你取消了本次支付</div>
          <button
            onClick={handlePay}
            className="w-full rounded-xl py-3 text-sm font-semibold mb-3"
            style={{ background: 'var(--semi-color-primary)', color: '#fff', border: 'none', height: '48px' }}
          >
            重新支付
          </button>
          <button
            onClick={() => { if (window.WeixinJSBridge) window.WeixinJSBridge.call('closeWindow'); }}
            className="w-full rounded-xl py-3 text-sm font-semibold"
            style={{ background: 'var(--semi-color-fill-1)', color: 'var(--semi-color-text-1)', border: `1px solid var(--semi-color-border)`, height: '48px' }}
          >
            关闭页面
          </button>
        </div>
      </div>
    );
  }

  if (state === 'success') {
    return (
      <div className="flex items-center justify-center min-h-screen p-4" style={{ background: 'var(--semi-color-bg-0)' }}>
        <div className="text-center max-w-sm w-full">
          <div className="mb-4" style={{ fontSize: '64px' }}>✅</div>
          <div className="mb-2" style={{ color: 'var(--semi-color-success)', fontSize: '20px', fontWeight: 700 }}>支付成功</div>
          <div className="mb-2" style={{ color: 'var(--semi-color-text-2)', fontSize: '14px' }}>
            {checkout?.pool_name} · {periodLabel(checkout?.period_months)}
          </div>
          <div className="mb-6" style={{ color: 'var(--semi-color-text-2)', fontSize: '14px' }}>已成功续期</div>
          <button
            onClick={() => { if (window.WeixinJSBridge) window.WeixinJSBridge.call('closeWindow'); }}
            className="w-full rounded-xl py-3 text-sm font-semibold"
            style={{ background: 'var(--semi-color-primary)', color: '#fff', border: 'none', height: '48px' }}
          >
            完成
          </button>
        </div>
      </div>
    );
  }

  const hasDiscount = checkout && checkout.base_amount_fen > checkout.amount_fen;
  const savings = hasDiscount ? checkout.base_amount_fen - checkout.amount_fen : 0;

  return (
    <div className="flex items-center justify-center min-h-screen p-4" style={{ background: 'var(--semi-color-bg-0)' }}>
      <div className="text-center max-w-sm w-full">
        <div className="mb-6" style={{ color: 'var(--semi-color-text-2)', fontSize: '12px' }}>
          🛡️ 安全支付
        </div>

        <div className="mb-2" style={{ color: 'var(--semi-color-primary)', fontSize: '48px', fontWeight: 700, lineHeight: 1.2 }}>
          ¥{formatFen(checkout?.amount_fen)}
        </div>

        <div className="mb-1" style={{ color: 'var(--semi-color-text-1)', fontSize: '15px' }}>
          {checkout?.pool_name || ''} · {periodLabel(checkout?.period_months)}
        </div>

        {hasDiscount ? (
          <div className="mb-1" style={{ color: 'var(--semi-color-text-2)', fontSize: '13px' }}>
            <span style={{ textDecoration: 'line-through' }}>原价 ¥{formatFen(checkout?.base_amount_fen)}</span>
            {' · '}
            <span style={{ color: 'var(--semi-color-success)' }}>省 ¥{formatFen(savings)}</span>
          </div>
        ) : null}

        <div className="mb-8" style={{ color: 'var(--semi-color-text-3)', fontSize: '13px' }}>
          支付方式：微信支付
        </div>

        <button
          onClick={handlePay}
          className="w-full rounded-xl py-3 text-sm font-semibold mb-4"
          style={{ background: 'var(--semi-color-primary)', color: '#fff', border: 'none', height: '48px' }}
        >
          确认支付
        </button>

        <div style={{ color: 'var(--semi-color-text-3)', fontSize: '12px' }}>
          支付即表示同意服务条款
        </div>
      </div>
    </div>
  );
}