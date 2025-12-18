<template>
  <div class="values-step">
    <div class="values-header">
      <h3>{{ t('suseai.wizard.form.valuesYaml', 'values.yaml') }}</h3>
    </div>

    <YamlEditor
      v-model:value="localValues"
      :as-object="true"
      class="values-editor"
      @update:value="handleUpdate"
    />

    <Banner v-if="versionDirty" color="info" class="mt-10">
      {{ t('suseai.wizard.form.versionDirtyWarning', 'Version changed after you edited values. Click "Load defaults from chart" to apply defaults for the new version, or continue with your customizations.') }}
    </Banner>
  </div>
</template>

<script lang="ts" setup>
import { computed } from 'vue';
import YamlEditor from '@shell/components/YamlEditor';
import { Banner } from '@components/Banner';

interface Props {
  values: Record<string, any>;
  chartRepo: string;
  chartName: string;
  chartVersion: string;
  loadingValues: boolean;
  versionDirty: boolean;
}

interface Emits {
  (e: 'update:values', values: Record<string, any>): void;
  (e: 'load-defaults'): void;
  (e: 'values-edited'): void;
}

const props = defineProps<Props>();
const emit = defineEmits<Emits>();

// Simple fallback function for translations
const t = (key: string, fallback: string) => fallback;

const localValues = computed({
  get: () => {
    console.log('[SUSE-AI DEBUG] ValuesStep localValues get:', {
      hasValues: !!props.values,
      valuesKeys: Object.keys(props.values || {}),
      valuesLength: Object.keys(props.values || {}).length,
      valuesSample: JSON.stringify(props.values, null, 2).substring(0, 200)
    });
    return props.values;
  },
  set: (value: Record<string, any>) => {
    console.log('[SUSE-AI DEBUG] ValuesStep localValues set:', Object.keys(value || {}));
    emit('update:values', value);
  }
});

function handleUpdate(value: Record<string, any>) {
  emit('update:values', value);
  emit('values-edited');
}
</script>

<style scoped>
.values-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.values-header h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--body-text);
}

.mt-10 {
  margin-top: 10px;
}

.mr-5 {
  margin-right: 5px;
}

.values-editor {
  min-height: 400px;
}
</style>