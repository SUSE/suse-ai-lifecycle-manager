<template>
  <div class="target-step">
    <template v-if="isInstallMode">
      <label class="lbl">Select Target Cluster(s)</label>
      <ClusterResourceTable
        :multi-select="true"
        :selected-clusters="clusters"
        @update:selected-clusters="$emit('update:clusters', $event)"
        :app-slug="appSlug"
        :app-name="appName"
        :disabled="false"
      />
    </template>
    <template v-else>
      <label class="lbl">Target Cluster</label>
      <ClusterSelect
        :model-value="clusters[0] || ''"
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
  clusters: string[]; // Array-based selection for both modes
  appSlug: string;
  appName: string;
}

interface Emits {
  (e: 'update:clusters', clusters: string[]): void;
}

const props = defineProps<Props>();
defineEmits<Emits>();

const isInstallMode = computed(() => props.mode === 'install');
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
  color: var(--body-text, #111827); 
  margin-bottom: 6px; 
}

.hint { 
  font-size: 12px; 
  color: var(--muted, #64748b); 
  margin-top: 8px; 
}
</style>
