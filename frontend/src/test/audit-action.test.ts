import { describe, expect, it } from 'vitest';

import { describeAuditAction } from '@/pages/audit/auditAction';

describe('describeAuditAction', () => {
  it.each([
    ['login', { section: 'login', operation: '' }],
    ['POST /clients/update/:email', { section: 'clients', operation: 'update' }],
    ['POST /inbounds/:id/resetTraffic', { section: 'inbounds', operation: 'resetTraffic' }],
    ['POST /clients/groups/bulkAdd', { section: 'clients', operation: 'groups › bulkAdd' }],
    ['DELETE /clients/hwids/:email/:id', { section: 'clients', operation: 'DELETE hwids' }],
    ['POST /sub-balancers', { section: 'sub-balancers', operation: 'POST' }],
    ['POST /xray/warp/:action', { section: 'xray', operation: 'warp' }],
  ])('splits %j into a section and an operation', (action, want) => {
    expect(describeAuditAction(action)).toEqual(want);
  });
});
