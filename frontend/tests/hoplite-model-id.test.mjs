import test from 'node:test';
import assert from 'node:assert/strict';

import { resolveHopliteProjectModelID } from '../src/lib/hoplite-model-id.ts';

test('secondary account Copy/Test resolver uses its advertised qualified ID', () => {
  const models = [
    { id: 'hoplite-agent/shared', hoplite_project_id: 'shared', hoplite_account_id: 'hoplite-cloud-agent', catalog_eligibility: 'project-default' },
    { id: 'hoplite-agent/v2/secondary/shared', hoplite_project_id: 'shared', hoplite_account_id: 'hoplite-cloud-agent-secondary', catalog_eligibility: 'project-default' }
  ];

  const defaultID = resolveHopliteProjectModelID(models, '', 'shared');
  const secondaryID = resolveHopliteProjectModelID(models, 'hoplite-cloud-agent-secondary', 'shared');

  assert.equal(defaultID, 'hoplite-agent/shared');
  assert.equal(secondaryID, 'hoplite-agent/v2/secondary/shared');
  assert.notEqual(secondaryID, defaultID);
});

test('secondary account resolver fails closed when qualified model is absent', () => {
  assert.equal(resolveHopliteProjectModelID([], 'hoplite-cloud-agent-secondary', 'shared'), '');
});
