<template>
  <div class="cluster-resource-table">
    <!-- Requirements display -->
    <div v-if="appRequirements" class="requirements-info" :class="{ 'requirements-estimated': isUsingDefaultRequirements }">
      <span class="requirements-label">
        {{ isUsingDefaultRequirements ? 'Estimated Requirements:' : 'Requirements:' }}
      </span>
      <span class="requirements-text">
        {{ appRequirements.cpu }} CPU cores • 
        {{ appRequirements.memory }}GB Memory
        <template v-if="appRequirements.gpu"> • {{ appRequirements.gpu }}GB GPU Memory</template>
        • {{ appRequirements.storage }}GB Storage
      </span>
      <div v-if="isUsingDefaultRequirements" class="requirements-note">
        ⚠️ Using conservative estimates - actual requirements may vary
      </div>
    </div>

    <!-- Loading state -->
    <div v-if="loading" class="table-loading">
      <div class="loading-text">🔄 Checking cluster resources...</div>
    </div>

    <!-- Error state -->
    <div v-else-if="error" class="table-error">
      <div class="error-text">⚠️ {{ error }}</div>
      <div class="error-hint">Showing basic cluster information only</div>
    </div>

    <!-- Cluster selection table -->
    <div v-else-if="clusters.length > 0" class="table-container">
      <table class="cluster-table table">
        <thead>
          <tr>
            <th class="col-select"></th>
            <th class="col-cluster">Cluster</th>
            <th class="col-nodes">Nodes</th>
            <th class="col-cpu">CPU</th>
            <th class="col-memory">Memory</th>
            <th class="col-gpu">GPU</th>
            <th class="col-status">Status</th>
          </tr>
        </thead>
        <tbody>
          <tr 
            v-for="cluster in clusters" 
            :key="cluster.clusterId"
            class="cluster-row"
            :class="{ 
              'row-selected': selectedCluster === cluster.clusterId,
              'row-compatible': cluster.status === 'compatible',
              'row-limited': cluster.status === 'limited',
              'row-insufficient': cluster.status === 'insufficient',
              'row-error': cluster.status === 'error'
            }"
            @click="selectCluster(cluster.clusterId)"
          >
            <td class="col-select">
              <input 
                type="radio" 
                :name="`cluster-select-${tableId}`"
                :value="cluster.clusterId"
                :checked="selectedCluster === cluster.clusterId"
                @change="selectCluster(cluster.clusterId)"
                class="cluster-radio"
              />
            </td>
            <td class="col-cluster">
              <div class="cluster-name">{{ cluster.name }}</div>
              <div v-if="cluster.clusterId !== cluster.name" class="cluster-id">({{ cluster.clusterId }})</div>
            </td>
            <td class="col-nodes">
              <span v-if="cluster.nodeCount > 0">{{ cluster.nodeCount }}</span>
              <span v-else class="no-resource">—</span>
            </td>
            <td class="col-cpu">
              <div v-if="cluster.resources.cpu.total > 0" class="resource-bar-container">
                <div class="resource-bar-track">
                  <div 
                    class="resource-bar-fill" 
                    :class="getResourceBarClass(cluster.resources.cpu.used, cluster.resources.cpu.total)"
                    :style="{ width: `${Math.min(100, (cluster.resources.cpu.used / cluster.resources.cpu.total) * 100)}%` }"
                  ></div>
                </div>
                <div class="resource-percentage">
                  {{ Math.ceil((cluster.resources.cpu.used / cluster.resources.cpu.total) * 100) }}%
                </div>
              </div>
              <span v-else class="no-resource">Unknown</span>
            </td>
            <td class="col-memory">
              <div v-if="cluster.resources.memory.total > 0" class="resource-bar-container">
                <div class="resource-bar-track">
                  <div 
                    class="resource-bar-fill" 
                    :class="getResourceBarClass(cluster.resources.memory.used, cluster.resources.memory.total)"
                    :style="{ width: `${Math.min(100, (cluster.resources.memory.used / cluster.resources.memory.total) * 100)}%` }"
                  ></div>
                </div>
                <div class="resource-percentage">
                  {{ Math.ceil((cluster.resources.memory.used / cluster.resources.memory.total) * 100) }}%
                </div>
              </div>
              <span v-else class="no-resource">Unknown</span>
            </td>
            <td class="col-gpu">
              <div v-if="cluster.resources.gpu && cluster.resources.gpu.total > 0" class="resource-bar-container">
                <div class="resource-bar-track">
                  <div 
                    class="resource-bar-fill" 
                    :class="getResourceBarClass(cluster.resources.gpu.used, cluster.resources.gpu.total)"
                    :style="{ width: `${Math.min(100, (cluster.resources.gpu.used / cluster.resources.gpu.total) * 100)}%` }"
                  ></div>
                </div>
                <div class="resource-percentage">
                  {{ Math.ceil((cluster.resources.gpu.used / cluster.resources.gpu.total) * 100) }}%
                </div>
              </div>
              <span v-else class="no-resource">—</span>
            </td>
            <td class="col-status">
              <span class="status-icon" :title="cluster.statusMessage">
                {{ getStatusIcon(cluster.status) }}
              </span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- No clusters state -->
    <div v-else class="no-clusters">
      <div class="no-clusters-text">No clusters available</div>
      <div class="no-clusters-hint">Check your cluster connections and permissions</div>
    </div>

    <!-- Selected cluster details -->
    <div v-if="selectedCluster && selectedClusterInfo" class="selected-info">
      <div class="selected-header">Selected: {{ selectedClusterInfo.name }}</div>
      <div class="selected-details" :class="`details-${selectedClusterInfo.status}`">
        <div v-if="selectedClusterInfo.status === 'compatible'" class="status-message">
          ✅ This cluster meets all requirements
        </div>
        <div v-else-if="selectedClusterInfo.status === 'limited'" class="status-message">
          ⚠️ {{ selectedClusterInfo.statusMessage || 'Limited compatibility' }}
        </div>
        <div v-else-if="selectedClusterInfo.status === 'insufficient'" class="status-message">
          ❌ {{ selectedClusterInfo.statusMessage || 'Insufficient resources' }}
        </div>
        <div v-else-if="selectedClusterInfo.status === 'error'" class="status-message">
          ❓ {{ selectedClusterInfo.statusMessage || 'Unable to check resources' }}
          <div class="status-hint">You can still install, but resource requirements cannot be verified.</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts">
import { defineComponent, ref, computed, onMounted, watch, PropType, getCurrentInstance } from 'vue';
import { 
  getAllClusterResourceMetrics, 
  checkAppCompatibility, 
  getAppResourceRequirements,
  getDefaultAppResourceRequirements,
  type ClusterResourceSummary 
} from '../../services/cluster-resources';

let tableIdCounter = 0;

export default defineComponent({
  name: 'ClusterResourceTable',
  props: {
    modelValue: { type: String, default: '' },
    appSlug: { type: String, required: true },
    appName: { type: String, default: '' },
    disabled: { type: Boolean, default: false }
  },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    const tableId = ++tableIdCounter; // Unique ID for radio button grouping
    const loading = ref(true);
    const error = ref<string | null>(null);
    const clusters = ref<ClusterResourceSummary[]>([]);
    
    const appRequirements = computed(() => {
      const profile = getAppResourceRequirements(props.appSlug);
      if (profile) {
        return profile.requirements;
      } else {
        // Use defaults if no profile exists
        const defaultProfile = getDefaultAppResourceRequirements(props.appSlug, props.appName);
        return defaultProfile.requirements;
      }
    });

    const isUsingDefaultRequirements = computed(() => {
      return !getAppResourceRequirements(props.appSlug);
    });

    const selectedCluster = computed({
      get: () => props.modelValue,
      set: (value: string) => emit('update:modelValue', value)
    });

    const selectedClusterInfo = computed(() => {
      return clusters.value.find(c => c.clusterId === selectedCluster.value);
    });

    async function loadClusterResources() {
      // appRequirements.value will always have a value now (either real or default)
      // so we don't need to check for null here anymore

      try {
        loading.value = true;
        error.value = null;
        
        const vm = getCurrentInstance()!.proxy as any;
        const store = vm.$store;
        
        console.log('[SUSE-AI] ClusterResourceTable: Loading cluster resources...');
        const clusterSummaries = await getAllClusterResourceMetrics(store);
        
        // Check compatibility for each cluster
        const clustersWithCompatibility = clusterSummaries.map(cluster => 
          checkAppCompatibility(props.appSlug, cluster, props.appName)
        );
        
        clusters.value = clustersWithCompatibility;
        console.log('[SUSE-AI] ClusterResourceTable: Loaded', clusters.value.length, 'clusters');
        
        // Auto-select cluster if only one is available or if one is clearly the best choice
        if (clustersWithCompatibility.length === 1 && !selectedCluster.value) {
          const cluster = clustersWithCompatibility[0];
          if (cluster.clusterId) {
            selectedCluster.value = cluster.clusterId;
            console.log('[SUSE-AI] ClusterResourceTable: Auto-selected single cluster:', selectedCluster.value);
          }
        } else if (clustersWithCompatibility.length > 1 && !selectedCluster.value) {
          // Try to auto-select the best compatible cluster
          const compatible = clustersWithCompatibility.filter(c => c.status === 'compatible');
          if (compatible.length === 1) {
            selectedCluster.value = compatible[0].clusterId;
            console.log('[SUSE-AI] ClusterResourceTable: Auto-selected compatible cluster:', selectedCluster.value);
          } else if (compatible.length > 1) {
            // Select the first compatible cluster (could be enhanced with better logic)
            selectedCluster.value = compatible[0].clusterId;
            console.log('[SUSE-AI] ClusterResourceTable: Auto-selected first compatible cluster:', selectedCluster.value);
          } else {
            // No compatible clusters, but we still want to select something so user can proceed
            // Select first cluster even if it has resource issues
            selectedCluster.value = clustersWithCompatibility[0].clusterId;
            console.log('[SUSE-AI] ClusterResourceTable: Auto-selected first available cluster:', selectedCluster.value);
          }
        }
      } catch (e: any) {
        console.error('[SUSE-AI] ClusterResourceTable: Failed to load cluster resources:', e);
        error.value = e.message || 'Failed to load cluster information';
        
        // Try to load basic cluster list as fallback
        try {
          const vm = getCurrentInstance()!.proxy as any;
          const store = vm.$store;
          const basicClusters = await store.dispatch('management/findAll', { type: 'cluster' });
          clusters.value = (basicClusters || []).map((c: any) => ({
            clusterId: c.id,
            name: c.name || c.id,
            nodeCount: 0,
            resources: { cpu: { used: 0, total: 0 }, memory: { used: 0, total: 0 } },
            status: 'error' as const,
            statusMessage: 'Resource information unavailable',
            storageClasses: [],
            lastUpdated: new Date(),
            nodes: []
          }));
        } catch (fallbackError) {
          console.error('[SUSE-AI] ClusterResourceTable: Fallback also failed:', fallbackError);
        }
      } finally {
        loading.value = false;
      }
    }

    function selectCluster(clusterId: string) {
      if (!props.disabled) {
        selectedCluster.value = clusterId;
      }
    }

    function getStatusIcon(status: ClusterResourceSummary['status']): string {
      switch (status) {
        case 'compatible': return '✅';
        case 'limited': return '⚠️';
        case 'insufficient': return '❌';
        case 'checking': return '🔄';
        case 'error': return '❓';
        default: return '❓';
      }
    }

    function getResourceBarClass(used: number, total: number): string {
      if (total === 0) return 'resource-bar-low';
      const percentage = (used / total) * 100;
      if (percentage >= 90) return 'resource-bar-critical';
      if (percentage >= 70) return 'resource-bar-high';
      return 'resource-bar-low';
    }

    onMounted(() => {
      loadClusterResources();
    });

    // Watch for app changes and reload
    watch(() => props.appSlug, () => {
      loadClusterResources();
    });

    return {
      tableId,
      loading,
      error,
      clusters,
      appRequirements,
      isUsingDefaultRequirements,
      selectedCluster,
      selectedClusterInfo,
      selectCluster,
      getStatusIcon,
      getResourceBarClass
    };
  }
});
</script>

<style scoped>
.cluster-resource-table {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.requirements-info {
  padding: 12px 16px;
  background: var(--box-bg, var(--body-bg));
  border: 1px solid var(--border, #e2e8f0);
  border-radius: 8px;
  font-size: 14px;
}

.requirements-label {
  font-weight: 600;
  color: var(--body-text, #111827);
}

.requirements-text {
  color: var(--muted, #64748b);
}

.requirements-estimated {
  background: var(--warning-banner-bg, rgba(245, 158, 11, 0.15));
  border-color: var(--warning-border, #f59e0b);
}

.requirements-note {
  font-size: 12px;
  color: var(--warning, #d97706);
  margin-top: 6px;
  font-style: italic;
}

.table-loading,
.table-error,
.no-clusters {
  padding: 24px;
  text-align: center;
  color: var(--muted, #64748b);
  border: 1px solid var(--border, #e2e8f0);
  border-radius: 8px;
  background: var(--box-bg, var(--body-bg));
}

.error-text {
  color: var(--error, #dc2626);
  font-weight: 600;
  margin-bottom: 4px;
}

.error-hint {
  font-size: 12px;
  color: var(--muted, #64748b);
}

.no-clusters-text {
  font-weight: 600;
  margin-bottom: 4px;
}

.no-clusters-hint {
  font-size: 12px;
}

.table-container {
  border: 1px solid var(--border, #e2e8f0);
  border-radius: var(--border-radius-lg, 8px);
  overflow: hidden;
  background: var(--body-bg);
  box-shadow: 0 2px 4px rgba(15, 23, 42, 0.05);
}

.cluster-table {
  width: 100%;
  border-collapse: collapse;
  background: inherit;
}

.cluster-table th {
  background: var(--sortable-table-header-bg);
  border-bottom: 1px solid var(--border);
  padding: 12px;
  text-align: left;
  font-size: 13px;
  font-weight: 600;
  color: var(--body-text);
}

.cluster-table th.col-select {
  width: 40px;
  text-align: center;
}

.cluster-table th.col-cluster {
  text-align: left;
  width: auto;
  min-width: 120px;
}

.cluster-table th.col-nodes {
  width: 60px;
  text-align: center;
}

.cluster-table th.col-cpu,
.cluster-table th.col-memory,
.cluster-table th.col-gpu {
  width: 120px;
  text-align: center;
}

.cluster-table th.col-status {
  width: 60px;
  text-align: center;
}

.cluster-row {
  cursor: pointer;
  transition: background-color 0.15s ease;
}

.cluster-row:hover {
  background: var(--sortable-table-accent-bg);
}

.cluster-row.row-selected {
  background: var(--primary-banner-bg, rgba(59, 130, 246, 0.15));
}

.cluster-table td {
  padding: 12px;
  border-bottom: 1px solid var(--border);
  text-align: left;
  font-size: 14px;
  vertical-align: middle;
}

.cluster-table tr:last-child td {
  border-bottom: none;
}


.cluster-table td.col-select,
.cluster-table td.col-nodes,
.cluster-table td.col-cpu,
.cluster-table td.col-memory,
.cluster-table td.col-gpu,
.cluster-table td.col-status {
  text-align: center;
}

.cluster-name {
  font-weight: 600;
  color: var(--body-text, #111827);
}

.cluster-id {
  font-size: 12px;
  color: var(--muted, #64748b);
}

.resource-bar-container {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 100px;
}

.resource-bar-track {
  flex: 1;
  height: 6px;
  background: var(--progress-bg, #e2e8f0);
  border-radius: 3px;
  overflow: hidden;
  min-width: 60px;
}

.resource-bar-fill {
  height: 100%;
  border-radius: 3px;
  transition: width 0.3s ease;
}

/* Color the bar based on usage percentage */
.resource-bar-fill.resource-bar-low {
  background: var(--success, #10b981);
}

.resource-bar-fill.resource-bar-high {
  background: var(--warning, #f59e0b);
}

.resource-bar-fill.resource-bar-critical {
  background: var(--error, #ef4444);
}

.resource-percentage {
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
  font-size: 12px;
  color: var(--body-text, #374151);
  font-weight: 600;
  min-width: 35px;
  text-align: right;
}

.no-resource {
  color: var(--muted, #9ca3af);
}

.status-icon {
  font-size: 16px;
  cursor: help;
  color: var(--body-text);
}

.cluster-radio {
  cursor: pointer;
}


.selected-info {
  padding: 12px 16px;
  border: 1px solid var(--border, #e2e8f0);
  border-radius: 8px;
  background: var(--box-bg, var(--body-bg));
}

.selected-header {
  font-weight: 600;
  color: var(--body-text, #111827);
  margin-bottom: 8px;
}

.selected-details {
  font-size: 14px;
}

.details-compatible .status-message {
  color: var(--success, #059669);
}

.details-limited .status-message {
  color: var(--warning, #d97706);
}

.details-insufficient .status-message {
  color: var(--error, #dc2626);
}

.details-error .status-message {
  color: var(--muted, #6b7280);
}

.status-hint {
  font-size: 12px;
  color: var(--muted, #9ca3af);
  margin-top: 4px;
}

/* Responsive design */
@media (max-width: 768px) {
  .cluster-table {
    font-size: 12px;
  }
  
  .cluster-table th,
  .cluster-table td {
    padding: 8px 4px;
  }
  
  .cluster-table th.col-nodes,
  .cluster-table th.col-cpu,
  .cluster-table th.col-memory,
  .cluster-table th.col-gpu {
    width: 100px;
  }
  
  .resource-bar-container {
    min-width: 80px;
    gap: 6px;
  }
  
  .resource-bar-track {
    min-width: 40px;
  }
  
  .resource-percentage {
    font-size: 11px;
    min-width: 30px;
  }
}
</style>
