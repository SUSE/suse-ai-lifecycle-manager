<script lang="ts" setup>
import { defineProps, withDefaults, ref, computed, onMounted, getCurrentInstance } from 'vue';
import yaml from 'js-yaml';
import { Banner } from '@components/Banner';
import Loading from '@shell/components/Loading';
import AsyncButton from '@shell/components/AsyncButton';
import BasicInfoStep from './wizard/BasicInfoStep.vue';
import TargetStep from './wizard/TargetStep.vue';
import ValuesStep from './wizard/ValuesStep.vue';
import ReviewStep from './wizard/ReviewStep.vue';
import {
  findChartInRepo,
  ensureNamespace,
  createOrUpgradeApp,
  listChartVersions,
  fetchChartDefaultValues,
  ensureRegistrySecretSimple,
  ensureServiceAccountPullSecret,
  ensurePullSecretOnAllSAs,
  waitForAppInstall,
  getClusters,
  getInstalledHelmDetails,
  inferClusterRepoForChart
} from '../../services/rancher-apps';
import { getRepoAuthForClusterRepo } from '../../services/repo-auth';
import { persistLoad, persistSave, persistClear } from '../../services/ui-persist';
import { fetchSuseAiApps, getClusterRepoNameFromUrl } from '../../services/app-collection';

const REPO_CLUSTER = 'local' as const;

type WizardMode = 'install' | 'manage';

type WizardForm = {
  release: string;
  namespace: string;
  cluster: string;
  clusters: string[];
  chartRepo: string;
  chartName: string;
  chartVersion: string;
  values: Record<string, any>;
};

interface Props {
  slug: string;
  mode?: WizardMode;
}

const props = withDefaults(defineProps<Props>(), {
  mode: 'install'
});

const vm = getCurrentInstance()!.proxy as any;
const store = vm.$store;
const router = vm.$router;
const route = vm.$route;

const loading = ref(true);
const submitting = ref(false);
const error = ref<string | null>(null);
const versions = ref<string[]>([]);
const loadingVersions = ref(false);
const loadingValues = ref(false);

const PKEY = `${props.mode}.${props.slug}`;
const TTL = 1000 * 60 * 60;

const form = ref<WizardForm>({
  release: props.slug,
  namespace: `${props.slug}-system`,
  cluster: '',
  clusters: [],
  chartRepo: '',
  chartName: props.slug,
  chartVersion: '',
  values: {}
});

// Mode and version computed properties - must be declared before wizardSteps
const isInstallMode = computed(() => props.mode === 'install');
const isManageMode = computed(() => props.mode === 'manage');

const versionOptions = computed(() =>
  (versions.value || []).map(v => ({ label: v, value: v }))
);

// Wizard step configuration for Rancher Wizard component
const wizardSteps = computed(() => [
  {
    name: 'basic-info',
    label: 'Basic Information',
    ready: true,
    weight: 1
  },
  {
    name: 'target',
    label: 'Target Cluster',
    ready: !!form.value.chartRepo && !!form.value.chartVersion && !loadingVersions.value,
    weight: 2
  },
  {
    name: 'values',
    label: 'Configuration',
    ready: (isInstallMode.value ? !!form.value.cluster : !!form.value.clusters.length) && !loadingValues.value,
    weight: 3
  },
  {
    name: 'review',
    label: 'Review',
    ready: (isInstallMode.value ? !!form.value.cluster : !!form.value.clusters.length),
    weight: 4
  }
]);

const currentStep = ref(0);

// Restore persisted state
const saved = persistLoad<{ step?: number; form?: Partial<WizardForm> }>(PKEY, {}, TTL);
if (saved.form) Object.assign(form.value, saved.form);
if (typeof saved.step === 'number') currentStep.value = Math.min(saved.step, wizardSteps.value.length - 1);

const wizardTitle = computed(() => isInstallMode.value ? 'Install' : 'Manage');

// Basic info form computed
const basicInfoForm = computed({
  get: () => ({
    release: form.value.release,
    namespace: form.value.namespace,
    chartRepo: form.value.chartRepo,
    chartName: form.value.chartName,
    chartVersion: form.value.chartVersion
  }),
  set: async (value) => {
    const oldRepo = form.value.chartRepo;
    const oldVersion = form.value.chartVersion;

    form.value.release = value.release;
    form.value.namespace = value.namespace;
    form.value.chartRepo = value.chartRepo;
    form.value.chartName = value.chartName;
    form.value.chartVersion = value.chartVersion;

    // Refresh versions if repo changed
    if (oldRepo !== value.chartRepo) {
      await refreshVersions();
    }

    // Load default values if version changed in install mode
    if (oldVersion !== value.chartVersion) {
      await onVersionChange();
    }
  }
});

onMounted(async () => {
  try {
    await initializeWizard();
  } catch (e) {
    error.value = `Failed to initialize: ${e.message || 'Unknown error'}`;
  } finally {
    loading.value = false;
  }
});

async function initializeWizard() {
  // Populate from URL parameters first
  populateFromUrlParams();

  if (isInstallMode.value) {
    await initializeInstallMode();
  } else {
    await initializeManageMode();
  }

  await refreshVersions();
}

function populateFromUrlParams() {
  const query = route?.query || {};

  if (isInstallMode.value) {
    // For install mode, use simple pre-population
    form.value.release = props.slug;
    form.value.namespace = `${props.slug}-system`;
    form.value.chartRepo = query.repo as string || '';
    form.value.chartName = props.slug;
  } else {
    // For manage mode, get from URL path parameters
    form.value.release = query.instanceName as string || props.slug;
    form.value.namespace = query.instanceNamespace as string || `${props.slug}-system`;
    form.value.cluster = query.instanceCluster as string || '';
  }
}

async function initializeInstallMode() {
  if (!store) return;

  // If repo is provided in query, use it
  if (form.value.chartRepo) {
    const guess = await findChartInRepo(store, REPO_CLUSTER, form.value.chartRepo, props.slug);
    if (guess) {
      form.value.chartName = guess.chartName;
      form.value.chartVersion = guess.version;
      return;
    }
  }

  // Try to find the app in SUSE AI apps collection
  const repo = await findRepoForApp(props.slug);
  if (repo) {
    form.value.chartRepo = repo;
    const guess = await findChartInRepo(store, REPO_CLUSTER, repo, props.slug);
    if (guess) {
      form.value.chartName = guess.chartName;
      form.value.chartVersion = guess.version;
    }
  }
}

async function findRepoForApp(slug: string): Promise<string | null> {
  if (!store) return null;

  try {
    // First try SUSE AI apps
    const suseAiApps = await fetchSuseAiApps(store);
    const staticApp = suseAiApps.find(app => app.slug_name === slug);

    if (staticApp?.repository_url) {
      const clusterRepoName = await getClusterRepoNameFromUrl(store, staticApp.repository_url);
      if (clusterRepoName) return clusterRepoName;
    }

    // Fallback to inference
    return await inferClusterRepoForChart(store, slug);
  } catch (e) {
    console.warn('Failed to find repo for app:', e);
    return null;
  }
}

async function initializeManageMode() {
  if (!store) throw new Error('Store not available');

  const targetCluster = form.value.cluster;

  if (!targetCluster) {
    throw new Error('No cluster specified for manage mode');
  }

  // Set clusters to only the current cluster context
  form.value.clusters = [targetCluster];

  // Load app details from the target cluster
  // If app doesn't exist, loadInstalledAppDetails will throw an error
  await loadInstalledAppDetails(targetCluster);
}

async function loadInstalledAppDetails(clusterId: string) {
  let foundValues = false;
  if (!store) return;

  try {
    const helmDetails = await getInstalledHelmDetails(store, clusterId, form.value.namespace, form.value.release);

    // Verify we got valid data - if app doesn't exist, chartName would be missing
    if (!helmDetails.chartName) {
      throw new Error(`App ${form.value.release} not found in cluster ${clusterId}`);
    }

    if (helmDetails.chartName) form.value.chartName = helmDetails.chartName;
    if (helmDetails.chartVersion) {
      form.value.chartVersion = helmDetails.chartVersion;
    }

    if (helmDetails.values && Object.keys(helmDetails.values).length > 0) {
      form.value.values = helmDetails.values;
      foundValues = true;
    }
  } catch (helmError) {
    console.warn('Failed to load app details from Helm:', helmError);
    throw helmError; // Re-throw to fail fast if app doesn't exist
  }

  // Infer repo if still unknown
  if (!form.value.chartRepo && form.value.chartName) {
    try {
      const repo = await inferClusterRepoForChart(store, form.value.chartName);
      if (repo) form.value.chartRepo = repo;
    } catch (e) {
      console.warn('Failed to infer repository:', e);
    }
  }

  // Log final results for debugging
  if (!foundValues) {
    console.warn('No values found for installed app:', {
      cluster: clusterId,
      namespace: form.value.namespace,
      release: form.value.release
    });
  }
}

async function refreshVersions() {
  if (!store || !form.value.chartRepo || !form.value.chartName) return;

  loadingVersions.value = true;
  try {
    const vs = await listChartVersions(store, REPO_CLUSTER, form.value.chartRepo, form.value.chartName);
    versions.value = Array.from(new Set(vs));

    // Set initial version if not already set
    if (!form.value.chartVersion && vs.length > 0) {
      form.value.chartVersion = vs[0];
    }

    // Load default values for install mode
    if (isInstallMode.value && form.value.chartVersion) {
      await loadDefaultValues();
    }
  } finally {
    loadingVersions.value = false;
  }
}

async function loadDefaultValues() {
  if (!store || !form.value.chartRepo || !form.value.chartName || !form.value.chartVersion) {
    return;
  }

  loadingValues.value = true;
  error.value = null;

  try {
    const valuesText = await fetchChartDefaultValues(
      store,
      REPO_CLUSTER,
      form.value.chartRepo,
      form.value.chartName,
      form.value.chartVersion
    );

    if (valuesText?.trim()) {
      form.value.values = (yaml.load(valuesText) as any) || {};
    } else {
      error.value = 'No default values found for the selected version.';
    }
  } catch (e: any) {
    error.value = e?.message || 'Failed to fetch default values.';
  } finally {
    loadingValues.value = false;
  }
}

// Handle version changes for install mode
async function onVersionChange() {
  if (isInstallMode.value && form.value.chartVersion) {
    await loadDefaultValues();
  }

  // Persist form state
  persistSave(PKEY, { step: currentStep.value, form: form.value });
}

// Simplified event handlers
function onValuesEdited() {
  // Persist form state when values are edited
  persistSave(PKEY, { step: currentStep.value, form: form.value });
}

// Wizard event handlers
function onWizardNext({ step }) {
  currentStep.value = step;
  persistSave(PKEY, { step: currentStep.value, form: form.value });
}

async function onWizardFinish() {
  await submit();
}

function onWizardCancel() {
  persistClear(PKEY);
  // Redirect to AppInstances page instead of Apps page
  router?.push({
    name: `c-cluster-suseai-app-instances`,
    params: {
      cluster: route?.params?.cluster,
      slug: props.slug
    },
    query: {
      repo: route.query.repo
    }
  });
}

async function submit() {
  try {
    submitting.value = true;
    error.value = null;

    if (!form.value.chartRepo || !form.value.chartName || !form.value.chartVersion) {
      error.value = 'Please set repository, chart and version.'; return;
    }
    
    if (isInstallMode.value && !form.value.cluster) {
      error.value = 'Please select a cluster.'; return;
    }
    
    if (isManageMode.value && !form.value.clusters.length) {
      error.value = 'Please select target cluster.'; return;
    }
    
    if (!store) { error.value = 'Store not available'; return; }

    const actionLabel = isInstallMode.value ? 'INSTALL' : 'UPGRADE';
    const targetClusters = isInstallMode.value ? [form.value.cluster] : form.value.clusters;
    
    console.log(`[SUSE-AI] ${actionLabel} start `, { 
      clusters: targetClusters, 
      ns: form.value.namespace, 
      release: form.value.release 
    });

    if (isInstallMode.value) {
      await performInstall();
    } else {
      await performUpgrade();
    }

    console.log(`[SUSE-AI] ${actionLabel} done `, { clusters: targetClusters });

    persistClear(PKEY);
    // Redirect to AppInstances page to show the newly installed/updated instance
    router?.push({
      name: `c-cluster-suseai-app-instances`,
      params: {
        cluster: route?.params?.cluster,
        slug: props.slug
      },
      query: {
        repo: route.query.repo
      }
    });
  } catch (e: any) {
    error.value = e?.message || `${finalButtonLabel.value} failed`;
  } finally {
    submitting.value = false;
  }
}

async function performInstall() {
  // Resolve creds from SELECTED ClusterRepo
  const repoCtx = await getRepoAuthForClusterRepo(store, form.value.chartRepo);
  const desiredSecretBase = repoCtx.secretName || `repo-${form.value.chartRepo}`;
  const hasRepoCredentials = !!repoCtx.auth?.username && !!repoCtx.auth?.password;

  const cid = form.value.cluster;
  await ensureNamespace(store, cid, form.value.namespace);

  let finalSecretName = '';
  if (hasRepoCredentials) {
    try {
      finalSecretName = await ensureRegistrySecretSimple(
        store, cid, form.value.namespace,
        repoCtx.registryHost, desiredSecretBase,
        repoCtx.auth!.username, repoCtx.auth!.password
      );
    } catch (e: any) {
      console.error('[SUSE-AI] pull-secret creation skipped:', e?.message || e);
    }
  }

  const v = JSON.parse(JSON.stringify(form.value.values || {}));

  // Only add imagePullSecrets if we have a secret name (i.e., repo has authentication)
  if (finalSecretName) {
    const addSecret = (arr: any): any[] => {
      const list = Array.isArray(arr) ? arr.slice() : [];
      const hasStr = list.some((e: any) => e === finalSecretName);
      const hasObj = list.some((e: any) => e && typeof e === 'object' && e.name === finalSecretName);
      if (!hasStr && !hasObj) list.push({ name: finalSecretName });
      return list;
    };
    v.global = v.global || {};
    v.global.imagePullSecrets = addSecret(v.global.imagePullSecrets);
    v.imagePullSecrets = addSecret(v.imagePullSecrets);

    const saCandidates = new Set<string>(['default']);
    const vs = (v as any).serviceAccount || {};
    if (typeof vs?.name === 'string' && vs.name.trim()) saCandidates.add(vs.name.trim());
    else if (vs.create === undefined || !!vs.create) saCandidates.add(form.value.release);
    for (const sa of saCandidates) {
      try { await ensureServiceAccountPullSecret(store, cid, form.value.namespace, sa, finalSecretName); }
      catch (e) { console.warn('[SUSE-AI] SA pull-secret attach (pre) failed', { sa, ns: form.value.namespace, e }); }
    }
  }

  console.log('[SUSE-AI] calling install ', { 
    cluster: cid, 
    repo: form.value.chartRepo, 
    chart: form.value.chartName, 
    version: form.value.chartVersion, 
    ns: form.value.namespace, 
    release: form.value.release, 
    values: v 
  });
  
  await createOrUpgradeApp(
    store, cid, form.value.namespace, form.value.release,
    { repoName: form.value.chartRepo, chartName: form.value.chartName, version: form.value.chartVersion },
    v,
    'install'
  );

  try {
    await waitForAppInstall(store, cid, form.value.namespace, form.value.release, 90_000);
  } catch (e: any) {
    console.error('[SUSE-AI] post-install app status (peek): ', { error: e?.message || e });
    throw new Error(`App resource did not appear in namespace ${form.value.namespace}. Check Rancher logs and ClusterRepo permissions.`);
  }

  if (finalSecretName) {
    for (let attempt = 1; attempt <= 5; attempt++) {
      try {
        await ensurePullSecretOnAllSAs(store, cid, form.value.namespace, finalSecretName);
        break;
      } catch (e) {
        if (attempt === 5) break;
        await new Promise(r => setTimeout(r, 2000));
      }
    }
  }
}

async function performUpgrade() {
  for (const cid of form.value.clusters) {
    await ensureNamespace(store, cid, form.value.namespace);
    await createOrUpgradeApp(
      store, cid, form.value.namespace, form.value.release,
      { repoName: form.value.chartRepo, chartName: form.value.chartName, version: form.value.chartVersion },
      form.value.values,
      'upgrade'
    );
  }
}

// Custom wizard navigation methods
function goToStep(stepIndex: number) {
  // Only allow navigation if step is ready or going backwards
  if (stepIndex <= currentStep.value || wizardSteps.value[stepIndex].ready) {
    currentStep.value = stepIndex;
  }
}

function nextStep() {
  if (currentStep.value < wizardSteps.value.length - 1 && wizardSteps.value[currentStep.value + 1].ready) {
    currentStep.value++;
  }
}

function previousStep() {
  if (currentStep.value > 0) {
    currentStep.value--;
  }
}
</script>

<template>
  <div class="install-steps pt-20 outlet">
    <Loading v-if="loading" />
    
    <div v-else class="custom-wizard">
      <!-- Fixed Header -->
      <div class="wizard-header">
        <h1>{{ (route.query.n as string) || props.slug }}</h1>
        <p class="text-muted">{{ wizardTitle }}</p>
      </div>

      <!-- Fixed Step Navigation -->
      <div class="wizard-nav">
        <div class="steps-container">
          <div 
            v-for="(step, index) in wizardSteps" 
            :key="step.name"
            class="step-item"
            :class="{
              'active': index === currentStep,
              'completed': index < currentStep,
              'disabled': !step.ready && index > currentStep
            }"
            @click="goToStep(index)"
          >
            <div class="step-number">
              <i v-if="index < currentStep" class="icon icon-checkmark" />
              <span v-else>{{ index + 1 }}</span>
            </div>
            <div class="step-label">{{ step.label }}</div>
          </div>
        </div>
      </div>

      <!-- Scrollable Content Area -->
      <div class="wizard-content-wrapper">
        <!-- Error Banner -->
        <Banner v-if="error" color="error" class="mb-20">
          {{ error }}
        </Banner>

        <!-- Step Content -->
        <div class="wizard-content">
          <!-- Step: Basic Information -->
          <BasicInfoStep
            v-if="currentStep === 0"
            v-model:form="basicInfoForm"
            :version-options="versionOptions"
            :loading-versions="loadingVersions"
          />

          <!-- Step: Target Cluster -->
          <TargetStep
            v-else-if="currentStep === 1"
            :mode="props.mode"
            v-model:cluster="form.cluster"
            v-model:clusters="form.clusters"
            :app-slug="props.slug"
            :app-name="(route.query.n as string) || props.slug"
          />

          <!-- Step: Configuration -->
          <ValuesStep
            v-else-if="currentStep === 2"
            v-model:values="form.values"
            :chart-repo="form.chartRepo"
            :chart-name="form.chartName"
            :chart-version="form.chartVersion"
            :loading-values="loadingValues"
            :version-dirty="false"
            @load-defaults="loadDefaultValues"
            @values-edited="onValuesEdited"
          />

          <!-- Step: Review -->
          <ReviewStep
            v-else-if="currentStep === 3"
            :mode="props.mode"
            :release="form.release"
            :namespace="form.namespace"
            :chart-repo="form.chartRepo"
            :chart-name="form.chartName"
            :chart-version="form.chartVersion"
            :cluster="form.cluster"
            :clusters="form.clusters"
            v-model:values="form.values"
            @values-edited="onValuesEdited"
          />
        </div>
      </div>

      <!-- Fixed Bottom Navigation Buttons -->
      <div class="wizard-buttons-fixed">
        <button
          v-if="currentStep > 0"
          class="btn role-secondary"
          @click="previousStep"
        >
          Previous
        </button>

        <div class="flex-spacer" />

        <button
          class="btn role-secondary mr-10"
          @click="onWizardCancel"
        >
          Cancel
        </button>

        <button
          v-if="currentStep === 0 || currentStep === 1 || currentStep === 2"
          class="btn role-primary"
          :disabled="!wizardSteps[currentStep].ready"
          @click="nextStep"
        >
          Next
        </button>

        <button
          v-else-if="currentStep === 3"
          class="btn role-primary"
          :disabled="!wizardSteps[currentStep].ready || submitting"
          @click="onWizardFinish"
        >
          <i v-if="submitting" class="icon icon-spinner icon-spin mr-5" />
          <span v-if="submitting">
            {{ props.mode === 'install' ? 'Installing...' : 'Saving...' }}
          </span>
          <span v-else>
            {{ props.mode === 'install' ? 'Install' : 'Save' }}
          </span>
        </button>
      </div>
    </div>
  </div>
</template>


<style scoped>
/* Button utilities */
.mr-5 {
  margin-right: 5px;
}

.icon-spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* Main wizard container - matches Rancher's main content areas */
.custom-wizard {
  background: var(--body-bg, #ffffff);
  max-width: 100%;
  width: 100%;
  height: calc(100vh - 140px);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* Header styling to match Rancher's page headers */
.wizard-header {
  flex-shrink: 0;
  padding: 20px 24px 16px 24px;
  background: var(--body-bg, #ffffff);
}

.wizard-header h1 {
  margin: 0 0 4px 0;
  font-size: 20px;
  font-weight: 600;
  color: var(--body-text, #111827);
  line-height: 1.2;
}

.wizard-header p {
  margin: 0;
  font-size: 14px;
  color: var(--muted, #6b7280);
  font-weight: 400;
}

/* Step navigation - matches Rancher's wizard pattern */
.wizard-nav {
  flex-shrink: 0;
  width: 100%;
  padding: 20px 24px;
  background: var(--body-bg, #ffffff);
}

.steps-container {
  display: flex;
  justify-content: space-between;
  position: relative;
  max-width: 100%;
  align-items: center;
}

.steps-container::before {
  content: '';
  position: absolute;
  top: 20px;
  left: 50px;
  right: 50px;
  height: 1px;
  background: var(--border, #f3f4f6);
  z-index: 0;
}

.step-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  cursor: pointer;
  flex: 1;
  max-width: 200px;
  position: relative;
  z-index: 1;
  transition: all 0.2s ease;
}

.step-item.disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.step-item:hover:not(.disabled) .step-number {
  transform: scale(1.05);
}

.step-number {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: var(--body-bg, #ffffff);
  border: 1px solid var(--border, #f3f4f6);
  color: var(--muted, #9ca3af);
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 500;
  font-size: 14px;
  margin-bottom: 8px;
  transition: all 0.2s ease;
}

.step-item.active .step-number {
  background: var(--primary, #2563eb);
  border-color: var(--primary, #2563eb);
  color: white;
}

.step-item.completed .step-number {
  background: var(--success, #16a34a);
  border-color: var(--success, #16a34a);
  color: white;
}

.step-label {
  font-size: 13px;
  text-align: center;
  color: var(--muted, #6b7280);
  font-weight: 400;
  line-height: 1.3;
}

.step-item.active .step-label {
  color: var(--primary, #2563eb);
  font-weight: 500;
}

.step-item.completed .step-label {
  color: var(--body-text, #111827);
}

/* Content area - matches Rancher's form containers */
.wizard-content-wrapper {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
  background: var(--body-bg, #ffffff);
}

.wizard-content {
  padding: 24px;
  background: var(--body-bg, #ffffff);
  margin: 0;
  min-height: 100%;
}

/* Bottom button bar - matches Rancher's action bars */
.wizard-buttons-fixed {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
  width: 100%;
  padding: 16px 24px;
  background: var(--body-bg, #ffffff);
}

.flex-spacer {
  flex: 1;
}

/* Button overrides to match Rancher's button styling */
.wizard-buttons-fixed .btn {
  height: 36px;
  padding: 0 16px;
  font-size: 14px;
  font-weight: 500;
  border-radius: 4px;
  transition: all 0.2s ease;
}

.wizard-buttons-fixed .btn.role-secondary {
  background: var(--body-bg, #ffffff);
  border: 1px solid var(--border, #d1d5db);
  color: var(--body-text, #111827);
}

.wizard-buttons-fixed .btn.role-secondary:hover {
  background: var(--accent-bg, #f9fafb);
  border-color: var(--border-hover, #9ca3af);
}

.wizard-buttons-fixed .btn.role-primary {
  background: var(--primary, #2563eb);
  border: 1px solid var(--primary, #2563eb);
  color: white;
}

.wizard-buttons-fixed .btn.role-primary:hover:not(:disabled) {
  background: var(--primary-hover, #1d4ed8);
  border-color: var(--primary-hover, #1d4ed8);
}

.wizard-buttons-fixed .btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

/* Utility classes */
.mb-20 {
  margin-bottom: 20px;
}
</style>