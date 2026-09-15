export type HopliteAdvertisedModel = {
  id: string;
  hoplite_project_id?: string;
  hoplite_account_id?: string;
  catalog_eligibility?: string;
};

export function resolveHopliteProjectModelID(
  models: HopliteAdvertisedModel[],
  accountID: string,
  projectID: string
): string {
  const expectedAccountID = accountID || 'hoplite-cloud-agent';
  const advertised = models.find((model) =>
    model.hoplite_project_id === projectID &&
    model.hoplite_account_id === expectedAccountID &&
    model.catalog_eligibility === 'project-default'
  );
  if (advertised) return advertised.id;
  if (accountID) return '';
  return `hoplite-agent/${projectID}`;
}
