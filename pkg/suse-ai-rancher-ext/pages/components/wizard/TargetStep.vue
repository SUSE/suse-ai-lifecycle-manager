<template>
  <div class="target-step">
    <template v-if="isInstallMode">
      <label class="lbl">Select Target Cluster</label>
      <ClusterResourceTable 
        v-model="localCluster"
        :app-slug="appSlug"
        :app-name="appName"
        :disabled="false"
      />
    </template>
    <template v-else>
      <label class="lbl">Target Cluster</label>
      <ClusterSelect
        v-model="displayCluster"
        :disabled="true"
      />
      <p class="hint">
        Changes will be applied only to the cluster in the current context and cannot be changed in Manage mode.
      </p>
    </template>
  </div>
</template>

<script lang="ts" setup>
import { computed } from 'vue';
import ClusterResourceTable from '../ClusterResourceTable.vue';
import ClusterSelect from '../ClusterSelect.vue';

interface Props {
  mode: 'install' | 'manage';
  cluster: string; // single cluster for install mode
  clusters: string[]; // multiple clusters for manage mode
  appSlug: string;
  appName: string;
}

interface Emits {
  (e: 'update:cluster', cluster: string): void;
  (e: 'update:clusters', clusters: string[]): void;
}

const props = defineProps<Props>();
const emit = defineEmits<Emits>();

const isInstallMode = computed(() => props.mode === 'install');

const localCluster = computed({
  get: () => props.cluster,
  set: (value: string) => emit('update:cluster', value)
});

// For manage mode, we show the first cluster but this represents all clusters
const displayCluster = computed({
  get() {
    if (isInstallMode.value) {
      return props.cluster;
    } else {
      return props.clusters[0] || '';
    }
  },
  set(value: string) {
    if (isInstallMode.value) {
      emit('update:cluster', value);
    } else {
      // In manage mode, this should not change, but we handle it gracefully
      if (value && !props.clusters.includes(value)) {
        emit('update:clusters', [value]);
      }
    }
  }
});
</script>

<style scoped>
.target-step {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.lbl { 
  display: block; 
  font-size: 12px; 
  color: #334155; 
  margin-bottom: 6px; 
}

.hint { 
  font-size: 12px; 
  color: #64748b; 
  margin-top: 8px; 
}
</style>