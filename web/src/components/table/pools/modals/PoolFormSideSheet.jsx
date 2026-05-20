import React from 'react';
import {
  Button,
  Input,
  Select,
  SideSheet,
  Space,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';

const { Title } = Typography;

const PoolFormSideSheet = ({
  visible,
  formData,
  setFormData,
  onSubmit,
  onCancel,
  t,
}) => {
  const isEdit = Number(formData?.id || 0) > 0;

  return (
    <SideSheet
      visible={visible}
      placement={isEdit ? 'right' : 'left'}
      onCancel={onCancel}
      closeIcon={null}
      title={
        <Space>
          <Tag color={isEdit ? 'blue' : 'green'} shape='circle'>
            {isEdit ? t('Update') : t('Create')}
          </Tag>
          <Title heading={4} className='m-0'>
            {isEdit ? t('Update Pool') : t('Create Pool')}
          </Title>
        </Space>
      }
      footer={
        <div className='flex justify-end bg-white'>
          <Space>
            <Button theme='solid' type='primary' onClick={onSubmit}>
              {isEdit ? t('Update') : t('Create')}
            </Button>
            <Button theme='light' onClick={onCancel}>
              {t('Cancel')}
            </Button>
          </Space>
        </div>
      }
      width={560}
    >
      <div className='p-4 space-y-3'>
        <Input
          placeholder='name'
          value={formData.name}
          onChange={(value) => setFormData((prev) => ({ ...prev, name: value }))}
        />
        <Input
          placeholder='description'
          value={formData.description}
          onChange={(value) =>
            setFormData((prev) => ({ ...prev, description: value }))
          }
        />
        <Select
          value={String(formData.status)}
          onChange={(value) =>
            setFormData((prev) => ({ ...prev, status: Number(value) }))
          }
        >
          <Select.Option value='1'>Enabled</Select.Option>
          <Select.Option value='2'>Disabled</Select.Option>
        </Select>
        <Input
          placeholder='monthly_price_cny (0 = no paid pool gate, decimals OK e.g. 1.50)'
          value={formData.monthly_price_cny_input ?? '0'}
          onChange={(value) =>
            setFormData((prev) => ({
              ...prev,
              monthly_price_cny_input: value,
            }))
          }
          onBlur={() =>
            setFormData((prev) => {
              const raw = String(prev.monthly_price_cny_input ?? '').trim();
              if (raw === '') {
                return {
                  ...prev,
                  monthly_price_cny: 0,
                  monthly_price_cny_input: '0',
                };
              }
              const n = Number.parseFloat(raw);
              if (!Number.isFinite(n) || n < 0) {
                return prev;
              }
              return {
                ...prev,
                monthly_price_cny: n,
                monthly_price_cny_input: raw,
              };
            })
          }
        />
        <Input
          placeholder='billing_currency (e.g. CNY)'
          value={formData.billing_currency || 'CNY'}
          onChange={(value) =>
            setFormData((prev) => ({ ...prev, billing_currency: value }))
          }
        />
        <Input
          placeholder='billing_period_seconds (default 2592000 = 30d)'
          value={String(formData.billing_period_seconds ?? 30 * 24 * 3600)}
          onChange={(value) =>
            setFormData((prev) => ({
              ...prev,
              billing_period_seconds:
                value === '' ? 30 * 24 * 3600 : parseInt(value, 10) || 30 * 24 * 3600,
            }))
          }
        />
      </div>
    </SideSheet>
  );
};

export default PoolFormSideSheet;
