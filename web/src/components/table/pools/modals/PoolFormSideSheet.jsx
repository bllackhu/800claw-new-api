import React from 'react';
import {
  Button,
  Divider,
  Input,
  InputNumber,
  Select,
  SideSheet,
  Space,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';

const { Title, Text } = Typography;

const PoolFormSideSheet = ({
  visible,
  formData,
  setFormData,
  onSubmit,
  onCancel,
  t,
}) => {
  const isEdit = Number(formData?.id || 0) > 0;

  const updateField = (field) => (value) =>
    setFormData((prev) => ({ ...prev, [field]: value }));

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
        <div>
          <Text type='tertiary' size='small' className='block mb-1'>Name</Text>
          <Input
            placeholder='name'
            value={formData.name}
            onChange={updateField('name')}
          />
        </div>
        <div>
          <Text type='tertiary' size='small' className='block mb-1'>Description</Text>
          <Input
            placeholder='description'
            value={formData.description}
            onChange={updateField('description')}
          />
        </div>
        <div>
          <Text type='tertiary' size='small' className='block mb-1'>Status</Text>
          <Select
            value={String(formData.status)}
            onChange={(value) =>
              setFormData((prev) => ({ ...prev, status: Number(value) }))
            }
          >
            <Select.Option value='1'>Enabled</Select.Option>
            <Select.Option value='2'>Disabled</Select.Option>
          </Select>
        </div>
        <div>
          <Text type='tertiary' size='small' className='block mb-1'>Monthly Price (CNY)</Text>
          <InputNumber
            placeholder='0 = no paid pool gate, decimals OK e.g. 1.50'
            style={{ width: '100%' }}
            value={Number(formData.monthly_price_cny_input ?? formData.monthly_price_cny ?? 0)}
            min={0}
            step={0.01}
            precision={2}
            hideButtons
            onChange={(value) => {
              const n = typeof value === 'number' && Number.isFinite(value) ? value : 0;
              setFormData((prev) => ({
                ...prev,
                monthly_price_cny: n,
                monthly_price_cny_input: String(n),
              }));
            }}
          />
        </div>
        <div>
          <Text type='tertiary' size='small' className='block mb-1'>Billing Currency</Text>
          <Input
            placeholder='e.g. CNY'
            value={formData.billing_currency || 'CNY'}
            onChange={updateField('billing_currency')}
          />
        </div>
        <div>
          <Text type='tertiary' size='small' className='block mb-1'>Billing Period (seconds)</Text>
          <Input
            placeholder='default 2592000 = 30d'
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
        <Divider margin='12px' />
        <Text type='secondary' size='small'>
          {t('Plan tier (optional)')}
        </Text>
        <div>
          <Text type='tertiary' size='small' className='block mb-1'>Display Name</Text>
          <Input
            placeholder='customer-facing label, falls back to name'
            value={formData.display_name || ''}
            onChange={updateField('display_name')}
          />
        </div>
        <div>
          <Text type='tertiary' size='small' className='block mb-1'>Plan Code</Text>
          <Input
            placeholder='e.g. lite, pro'
            value={formData.plan_code || ''}
            onChange={updateField('plan_code')}
          />
        </div>
        <div>
          <Text type='tertiary' size='small' className='block mb-1'>Plan Group</Text>
          <Input
            placeholder='e.g. standard - pools in same group can upgrade/downgrade'
            value={formData.plan_group || ''}
            onChange={updateField('plan_group')}
          />
        </div>
        <div>
          <Text type='tertiary' size='small' className='block mb-1'>Plan Tier</Text>
          <InputNumber
            placeholder='higher = more premium, e.g. Lite=10, Pro=20'
            style={{ width: '100%' }}
            value={Number(formData.plan_tier ?? 0)}
            min={0}
            step={1}
            precision={0}
            hideButtons
            onChange={(value) => {
              const n = typeof value === 'number' && Number.isFinite(value) ? value : 0;
              setFormData((prev) => ({ ...prev, plan_tier: n }));
            }}
          />
        </div>
        <div>
          <Text type='tertiary' size='small' className='block mb-1'>Display Order</Text>
          <InputNumber
            placeholder='ascending'
            style={{ width: '100%' }}
            value={Number(formData.display_order ?? 0)}
            min={0}
            step={1}
            precision={0}
            hideButtons
            onChange={(value) => {
              const n = typeof value === 'number' && Number.isFinite(value) ? value : 0;
              setFormData((prev) => ({ ...prev, display_order: n }));
            }}
          />
        </div>
        <Divider margin='12px' />
        <Text type='secondary' size='small'>
          {t('Rate Limit Mode')}
        </Text>
        <div>
          <Text type='tertiary' size='small' className='block mb-1'>{t('Rate Limit Mode')}</Text>
          <Select
            value={formData.rate_limit_mode || 'sliding'}
            onChange={updateField('rate_limit_mode')}
          >
            <Select.Option value='sliding'>{t('Sliding Window (default)')}</Select.Option>
            <Select.Option value='fixed'>{t('Fixed Window')}</Select.Option>
          </Select>
        </div>
      </div>
    </SideSheet>
  );
};

export default PoolFormSideSheet;