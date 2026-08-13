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

import React, { useEffect, useState } from 'react';
import {
  Button,
  Card,
  Col,
  Form,
  InputNumber,
  Row,
  Switch,
  Table,
  Tag,
  Typography,
  Banner,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess } from '../../helpers';
import { useTranslation } from 'react-i18next';

const { Text } = Typography;

const CAPABILITY_UNIT_LABELS = {
  count: '次',
  minutes: '分钟',
  seconds: '秒',
  images: '张',
};

const CapabilitySetting = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [capabilities, setCapabilities] = useState({});
  const [consumedTotal, setConsumedTotal] = useState({});
  const [formApi, setFormApi] = useState(null);

  const loadData = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/capability/settings');
      setCapabilities(res.data?.data?.capabilities || {});
      setConsumedTotal(res.data?.data?.consumed_total || {});
      formApi?.setValues(res.data?.data?.capabilities || {});
    } catch (err) {
      showError(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (formApi) {
      loadData();
    }
  }, [formApi]);

  const handleSave = async () => {
    setSaving(true);
    try {
      const values = formApi.getValues();
      const payload = {};
      Object.entries(values).forEach(([name, cfg]) => {
        payload[name] = {
          price_per_unit: Number(cfg?.price_per_unit ?? 0),
          purchased_total: Number(cfg?.purchased_total ?? 0),
          enabled: Boolean(cfg?.enabled),
        };
      });
      await API.put('/api/capability/settings', { capabilities: payload });
      showSuccess(t('保存成功'));
      await loadData();
    } catch (err) {
      showError(err);
    } finally {
      setSaving(false);
    }
  };

  const dataSource = Object.entries(capabilities).map(([name, cfg]) => {
    const consumed = consumedTotal[name] || 0;
    const purchased = Number(cfg?.purchased_total ?? 0);
    const remaining = purchased > 0 ? Math.max(0, purchased - consumed) : null;
    return { name, cfg, consumed, remaining, unit: CAPABILITY_UNIT_LABELS[cfg?.unit] || '次' };
  });

  return (
    <div className='p-4'>
      <Banner
        type='info'
        closeIcon={null}
        description={t(
          '能力系统：为独立端点能力（语音识别等）提供 Token 级授权与计量。开启某能力的"启用"开关后，未授权该能力的 Token 调用对应端点将返回 403，已授权 Token 按配置的计费模式（次数包 / 钱包扣费）计量。',
        )}
        style={{ marginBottom: 16 }}
      />
      <Card
        title={t('能力配置')}
        loading={loading}
        footer={
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
            <Button type='primary' loading={saving} onClick={handleSave}>
              {t('保存')}
            </Button>
          </div>
        }
      >
        <Form
          getFormApi={(api) => setFormApi(api)}
          style={{ maxWidth: '100%' }}
        >
          <Table
            columns={[
              {
                title: t('能力名称'),
                dataIndex: 'name',
                width: 180,
                render: (_, record) => (
                  <Text strong>{record.name}</Text>
                ),
              },
              {
                title: t('计量单位'),
                dataIndex: 'unit',
                width: 100,
                render: (_, record) => <Tag>{record.unit}</Tag>,
              },
              {
                title: t('零售价（USD/单位）'),
                dataIndex: 'price',
                width: 160,
                render: (_, record) => (
                  <Form.InputNumber
                    noLabel
                    field={`${record.name}.price_per_unit`}
                    min={0}
                    step={0.001}
                    precision={4}
                    style={{ width: 140 }}
                  />
                ),
              },
              {
                title: t('订阅包采购总量'),
                dataIndex: 'purchased',
                width: 160,
                render: (_, record) => (
                  <Form.InputNumber
                    noLabel
                    field={`${record.name}.purchased_total`}
                    min={0}
                    step={1}
                    style={{ width: 140 }}
                  />
                ),
              },
              {
                title: t('已消耗'),
                dataIndex: 'consumed',
                width: 100,
                render: (consumed) => <Text>{consumed}</Text>,
              },
              {
                title: t('剩余'),
                dataIndex: 'remaining',
                width: 100,
                render: (remaining) =>
                  remaining === null ? <Text type='tertiary'>-</Text> : <Text>{remaining}</Text>,
              },
              {
                title: t('启用授权'),
                dataIndex: 'enabled',
                width: 100,
                render: (_, record) => (
                  <Form.Switch noLabel field={`${record.name}.enabled`} checkedText={t('开')} uncheckedText={t('关')} />
                ),
              },
            ]}
            dataSource={dataSource}
            pagination={false}
            rowKey='name'
          />
        </Form>
      </Card>
    </div>
  );
};

export default CapabilitySetting;
