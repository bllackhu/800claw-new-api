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
  Form,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess } from '../../../../helpers';
import { useTranslation } from 'react-i18next';

const { Text } = Typography;

const MODE_LABELS = {
  count: '次数包',
  wallet: '钱包扣费',
};

const UNIT_LABELS = {
  count: '次',
  minutes: '分钟',
  seconds: '秒',
  images: '张',
};

const TokenCapabilitiesModal = ({ visible, token, onCancel }) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [grants, setGrants] = useState([]);
  const [registry, setRegistry] = useState([]);
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState(null);

  const loadData = async () => {
    if (!token?.id) return;
    setLoading(true);
    try {
      const res = await API.get(`/api/token/${token.id}/capabilities`);
      setGrants(res.data?.data?.capabilities || []);
      setRegistry(res.data?.data?.registry || []);
    } catch (err) {
      showError(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (visible) {
      setShowForm(false);
      setEditing(null);
      loadData();
    }
  }, [visible]);

  const handleSubmit = async (values) => {
    try {
      const mode = values.mode || 'count';
      const payload = {
        capability: values.capability,
        mode,
        granted: mode === 'count' ? Number(values.granted ?? 0) : 0,
      };
      await API.post(`/api/token/${token.id}/capabilities`, payload);
      showSuccess(t('保存成功'));
      setShowForm(false);
      setEditing(null);
      await loadData();
    } catch (err) {
      showError(err);
    }
  };

  const handleDelete = async (capability) => {
    try {
      await API.delete(`/api/token/${token.id}/capabilities/${capability}`);
      showSuccess(t('删除成功'));
      await loadData();
    } catch (err) {
      showError(err);
    }
  };

  const registryOptions = registry.map((r) => ({
    value: r.name,
    label: `${r.name}（${UNIT_LABELS[r.unit] || r.unit}）`,
  }));

  const currentCapabilities = new Set(grants.map((g) => g.capability));
  const availableOptions = registryOptions.filter((o) => !currentCapabilities.has(o.value));

  return (
    <Modal
      title={`${t('能力授权')} - ${token?.name || ''} (#${token?.id || ''})`}
      visible={visible}
      onCancel={onCancel}
      footer={null}
      width={680}
    >
      <Space vertical align='start' style={{ width: '100%' }}>
        <Card loading={loading} style={{ width: '100%' }}>
          <Table
            columns={[
              {
                title: t('能力'),
                dataIndex: 'capability',
                render: (v) => <Text strong>{v}</Text>,
              },
              {
                title: t('模式'),
                dataIndex: 'mode',
                render: (v) => <Tag color={v === 'count' ? 'green' : 'blue'}>{MODE_LABELS[v] || v}</Tag>,
              },
              {
                title: t('已授权'),
                dataIndex: 'granted',
                width: 100,
              },
              {
                title: t('剩余'),
                dataIndex: 'remaining',
                width: 100,
                render: (v) => (v == null ? '-' : <Text>{v}</Text>),
              },
              {
                title: t('操作'),
                render: (_, record) => (
                  <Space>
                    <Button size='small' onClick={() => { setEditing(record); setShowForm(true); }}>
                      {t('编辑')}
                    </Button>
                    <Popconfirm
                      title={t('确定删除该能力授权？')}
                      onConfirm={() => handleDelete(record.capability)}
                    >
                      <Button size='small' type='danger'>
                        {t('删除')}
                      </Button>
                    </Popconfirm>
                  </Space>
                ),
              },
            ]}
            dataSource={grants}
            pagination={false}
            rowKey='capability'
            empty={t('暂无能力授权')}
          />
        </Card>

        {!showForm ? (
          <Button
            type='primary'
            disabled={availableOptions.length === 0}
            onClick={() => { setEditing(null); setShowForm(true); }}
          >
            {t('添加能力')}
          </Button>
        ) : (
          <Card style={{ width: '100%' }}>
            <Form
              onSubmit={handleSubmit}
              initValues={
                editing
                  ? { capability: editing.capability, mode: editing.mode, granted: editing.granted }
                  : { mode: 'count', granted: 100 }
              }
            >
              <Form.Select
                field='capability'
                label={t('能力')}
                optionList={editing ? registryOptions.filter((o) => o.value === editing.capability || !currentCapabilities.has(o.value)) : availableOptions}
                disabled={!!editing}
                rules={[{ required: true, message: t('请选择能力') }]}
                style={{ width: '100%' }}
              />
              <Form.Select
                field='mode'
                label={t('计费模式')}
                optionList={[
                  { value: 'count', label: t('次数包（不扣钱包）') },
                  { value: 'wallet', label: t('钱包扣费（按零售价）') },
                ]}
                style={{ width: '100%' }}
              />
              <Form.InputNumber
                field='granted'
                label={t('授权次数')}
                min={0}
                step={1}
                style={{ width: '100%' }}
                extraText={t('count 模式生效；wallet 模式无需配置')}
              />
              <Space style={{ marginTop: 12 }}>
                <Button type='primary' htmlType='submit'>
                  {t('保存')}
                </Button>
                <Button onClick={() => { setShowForm(false); setEditing(null); }}>
                  {t('取消')}
                </Button>
              </Space>
            </Form>
          </Card>
        )}
      </Space>
    </Modal>
  );
};

export default TokenCapabilitiesModal;
