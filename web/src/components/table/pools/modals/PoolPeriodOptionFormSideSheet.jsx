import React from 'react';
import {
  Button,
  Input,
  InputNumber,
  SideSheet,
  Space,
  Switch,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';

const { Title, Text } = Typography;

const PoolPeriodOptionFormSideSheet = ({
  visible,
  formData,
  setFormData,
  onSubmit,
  onCancel,
  t,
}) => {
  const isEdit = Number(formData?.id || 0) > 0;
  const bp = Number(formData?.discount_ratio_bp ?? 10000) || 0;
  const bpPercent = bp > 0 ? (bp / 100).toFixed(2) : '0.00';

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
            {isEdit ? t('Update Period Option') : t('Create Period Option')}
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
          placeholder='pool_id'
          value={String(formData.pool_id ?? '')}
          onChange={(value) =>
            setFormData((prev) => ({ ...prev, pool_id: value }))
          }
        />
        <InputNumber
          placeholder='period_months (e.g. 1, 3, 6, 12)'
          style={{ width: '100%' }}
          value={Number(formData.period_months ?? 1)}
          min={1}
          step={1}
          precision={0}
          hideButtons
          onChange={(value) => {
            const n = typeof value === 'number' && Number.isFinite(value) ? value : 1;
            setFormData((prev) => ({ ...prev, period_months: n }));
          }}
        />
        <InputNumber
          placeholder='discount_ratio_bp (10000 = 100%, 9000 = 90%)'
          style={{ width: '100%' }}
          value={Number(formData.discount_ratio_bp ?? 10000)}
          min={1}
          max={10000}
          step={100}
          precision={0}
          hideButtons
          onChange={(value) => {
            const n =
              typeof value === 'number' && Number.isFinite(value) ? value : 10000;
            setFormData((prev) => ({ ...prev, discount_ratio_bp: n }));
          }}
        />
        <Text type='secondary' size='small'>
          {t('Effective price ratio')}: {bpPercent}%
        </Text>
        <InputNumber
          placeholder='sort_order (ascending)'
          style={{ width: '100%' }}
          value={Number(formData.sort_order ?? 0)}
          min={0}
          step={1}
          precision={0}
          hideButtons
          onChange={(value) => {
            const n = typeof value === 'number' && Number.isFinite(value) ? value : 0;
            setFormData((prev) => ({ ...prev, sort_order: n }));
          }}
        />
        <div className='flex items-center gap-2'>
          <Text type='secondary'>{t('Enabled')}</Text>
          <Switch
            checked={Boolean(formData.enabled)}
            onChange={(value) =>
              setFormData((prev) => ({ ...prev, enabled: Boolean(value) }))
            }
          />
        </div>
      </div>
    </SideSheet>
  );
};

export default PoolPeriodOptionFormSideSheet;
