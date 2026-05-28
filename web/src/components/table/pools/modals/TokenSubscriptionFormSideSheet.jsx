import React from 'react';
import {
  Button,
  DatePicker,
  Input,
  SideSheet,
  Space,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { toPeriodEndDateEndOfDay } from '../../../../hooks/pools/usePoolsData';

const { Title } = Typography;

const TokenSubscriptionFormSideSheet = ({
  visible,
  formData,
  setFormData,
  onSubmit,
  onCancel,
  onRevokeNow,
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
            {isEdit
              ? t('Update Token Subscription')
              : t('Create Token Subscription')}
          </Title>
        </Space>
      }
      footer={
        <div className='flex justify-end bg-white'>
          <Space>
            {isEdit ? (
              <Button type='warning' onClick={onRevokeNow}>
                {t('Revoke now')}
              </Button>
            ) : null}
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
          placeholder='token_id'
          value={String(formData.token_id ?? '')}
          disabled={isEdit}
          onChange={(value) =>
            setFormData((prev) => ({ ...prev, token_id: value }))
          }
        />
        <Input
          placeholder='pool_id'
          value={String(formData.pool_id ?? '')}
          disabled={isEdit}
          onChange={(value) =>
            setFormData((prev) => ({ ...prev, pool_id: value }))
          }
        />
        <DatePicker
          type='dateTime'
          density='compact'
          style={{ width: '100%' }}
          placeholder={t('Subscription period end')}
          value={formData.period_end_date}
          onChange={(date) =>
            setFormData((prev) => ({
              ...prev,
              period_end_date: date ? toPeriodEndDateEndOfDay(date) : null,
            }))
          }
        />
      </div>
    </SideSheet>
  );
};

export default TokenSubscriptionFormSideSheet;
