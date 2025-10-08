<template>
  <div class="review-step">
    <!-- Application Details -->
    <h3>Application Details</h3>
    <div class="details-grid">
      <div class="detail-item">
        <label>Release Name:</label>
        <span>{{ release }}</span>
      </div>
      <div class="detail-item">
        <label>Namespace:</label>
        <span>{{ namespace }}</span>
      </div>
      <div class="detail-item">
        <label>Repository:</label>
        <span>{{ chartRepo }}</span>
      </div>
      <div class="detail-item">
        <label>Chart:</label>
        <span>{{ chartName }}</span>
      </div>
      <div class="detail-item">
        <label>Version:</label>
        <span>{{ chartVersion }}</span>
      </div>
      <div class="detail-item full-width">
        <label>{{ isInstallMode ? 'Target Cluster:' : 'Target Cluster:' }}</label>
        <span>{{ clusterDisplay }}</span>
      </div>
    </div>

    <!-- Configuration -->
    <h3 class="mt-30">Configuration</h3>
    <YamlEditor 
      v-model:value="localValues"
      :as-object="true"
      class="values-editor"
      @update:value="$emit('values-edited')"
    />
  </div>
</template>

<script lang="ts" setup>
import { computed } from 'vue';
import YamlEditor from '@shell/components/YamlEditor';

interface Props {
  mode: 'install' | 'manage';
  release: string;
  namespace: string;
  chartRepo: string;
  chartName: string;
  chartVersion: string;
  cluster: string; // single cluster for install mode
  clusters: string[]; // multiple clusters for manage mode
  values: Record<string, any>;
}

interface Emits {
  (e: 'update:values', values: Record<string, any>): void;
  (e: 'values-edited'): void;
}

const props = defineProps<Props>();
const emit = defineEmits<Emits>();

const isInstallMode = computed(() => props.mode === 'install');

const clusterDisplay = computed(() => {
  if (isInstallMode.value) {
    return props.cluster || '— none —';
  } else {
    return props.clusters.join(', ') || '— none —';
  }
});

const localValues = computed({
  get: () => props.values,
  set: (value: Record<string, any>) => emit('update:values', value)
});
</script>

<style scoped>
.review-step {
  width: 100%;
  max-width: 100%;
  box-sizing: border-box;
  overflow: hidden;
}

.review-step h3 {
  margin: 0 0 16px 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--body-text);
}

.mt-30 {
  margin-top: 30px;
}

.details-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px 20px;
  margin-bottom: 20px;
  width: 100%;
  max-width: 100%;
  box-sizing: border-box;
}

.detail-item {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0; /* Allow flexbox to shrink */
  overflow: hidden;
}

.detail-item.full-width {
  grid-column: 1 / -1;
}

.detail-item label {
  font-weight: 500;
  color: var(--body-text);
  min-width: 80px;
  flex-shrink: 0;
  white-space: nowrap;
}

.detail-item span {
  color: var(--muted);
  font-family: monospace;
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 768px) {
  .details-grid {
    grid-template-columns: 1fr;
    gap: 8px;
  }
}

.values-editor {
  min-height: 300px;
}
</style>