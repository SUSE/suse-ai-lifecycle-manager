import type { IPlugin } from '@shell/core/types';
import suseaiStore from './store/suseai-common';
import {
  PRODUCT,
  BLANK_CLUSTER,
  SUSEAI_PRODUCT,
  VIRTUAL_TYPES,
  BASIC_TYPES,
  NAV_WEIGHTS,
  PAGE_TYPES
} from './config/suseai';
import type { RancherStore } from './types/rancher-types';

export { PRODUCT } from './config/suseai';

export function init($plugin: IPlugin, store: RancherStore) {
  const { product, virtualType, basicType, weightType } = $plugin.DSL(store, PRODUCT);

  // Register store modules following standard patterns
  store.registerModule?.(PRODUCT, suseaiStore);

  // Configure product following standard patterns
  product({
    icon:        'suseai',
    iconHeader:  require('./assets/SUSE-AI-Factory-Logo_pos-green-horizontal.svg'),
    inStore: SUSEAI_PRODUCT.inStore,
    isMultiClusterApp: true,
    showClusterSwitcher: false,
    weight: SUSEAI_PRODUCT.weight,
    to: {
      // Use the blank cluster ('_') so the extension is cluster-independent and
      // does not force-load the 'local' management cluster, which standard
      // (non-admin) users have no access to. Matches the nav items in config.
      name: `c-cluster-${PRODUCT}-${PAGE_TYPES.OVERVIEW}`,
      params: { product: PRODUCT, cluster: BLANK_CLUSTER },
      meta: { product: PRODUCT, cluster: BLANK_CLUSTER }
    }
  } as any);

  // Register virtual types following standard patterns.
  // ifHaveType/ifHaveVerb (set on admin-only items such as Settings) are
  // evaluated by the shell at nav-render time, so they correctly hide an item
  // from non-admins without breaking the rest of the nav.
  VIRTUAL_TYPES.forEach(vType => {
    virtualType({
      name:       vType.name,
      label:      vType.label,
      route:      vType.route,
      ...(vType.ifHaveType ? { ifHaveType: vType.ifHaveType } : {}),
      ...(vType.ifHaveVerb ? { ifHaveVerb: vType.ifHaveVerb } : {}),
    });
  });

  // Apply explicit sidebar ordering (higher weight = higher in list).
  Object.entries(NAV_WEIGHTS).forEach(([type, weight]) => {
    weightType(type, weight, true);
  });

  // Register basic types
  basicType(BASIC_TYPES);
}
