# SUSE AI Lifecycle Manager

SUSE AI Lifecycle Manager is a Rancher UI Extension for managing SUSE AI components across Kubernetes clusters. This extension provides a unified interface for installing, managing, and monitoring AI workloads in Rancher-managed clusters.

> **Note:** While this extension is open source (Apache 2.0), it requires an active [SUSE AI](https://www.suse.com/products/ai/) subscription to access the application catalog.

## Development

### Prerequisites

- Node.js 20+ and Yarn
- Access to a Rancher cluster
- Extension developer features enabled in Rancher

### Setup

1. **Clone and install dependencies:**
   ```bash
   git clone <repository-url>
   cd suse-ai-lifecycle-manager
   yarn install
   ```

2. **Build the extension:**
   ```bash
   yarn build-pkg suse-ai-lifecycle-manager
   ```

3. **Serve during development:**
   ```bash
   yarn serve-pkgs
   ```
   Copy the URL shown in the terminal.

4. **Load in Rancher:**
   - In Rancher, go to your user profile (top right) → Preferences
   - Enable "Extension Developer Features"
   - Navigate to Extensions from the side nav
   - Click the 3 dots (top right) → Developer Load
   - Paste the URL from step 3, select "Persist"
   - Reload the page

## Building for Production

```bash
yarn build-pkg suse-ai-lifecycle-manager --mode production
```

## Extension Catalog Container

- The container packages the SUSE AI Lifecycle Manager (Rancher UI Extension) into a single OCI container image.
- This container is:
   - Built and published during CI
   - Stored in GitHub Container Registry (GHCR)
   - Consumed by Rancher as an extension catalog source
- The catalog container allows:
   - Versioned releases
   - Immutable distribution
   - Simple rollout via container tags

 ### Versioning
- The catalog container tag is derived from the Git tag:
 
```
suse-ai-lifecycle-manager-<version> → ghcr.io/suse/suse-ai-lifecycle-manager:<version>
```

In the examples below, `<version>` refers to a published extension release (e.g. `0.2.0`).

Available catalog image versions are published in GitHub Container Registry:
https://github.com/SUSE/suse-ai-lifecycle-manager/pkgs/container/suse-ai-lifecycle-manager
 
### Container Structure
```
/home/plugin-server
└── plugin-contents/
    ├── files.txt
    ├── index.yaml
    └── plugin/
        ├── index.yaml
        ├── package.json
        ├── suse-ai-lifecycle-manager
            └── suse-ai-lifecycle-manager-<version>.tgz
        └── suse-ai-lifecycle-manager-<version>
            ├── files.txt
            └── plugin/
                └── <plugin source code>
```

### Consuming the Catalog in Rancher
- Add the catalog source in the Rancher Dashboard:
   1. Navigate to Extensions → Manage Extensions Catalog
   2. Import Extension Catalog → Use the Catalog Image Reference: `ghcr.io/suse/suse-ai-lifecycle-manager:<version>` → Press `Load`
   3. From the Extensions page, Go to Manage Repositories. Verify if the SUSE AI Rancher Extension repository has the `Active` state. If not, refresh the connection.
   4. Go back to Extensions and install SUSE AI Rancher Extension.

> NOTE: Replace `<version>` with a tag published in GitHub Container Registry.
> NOTE: Newly published catalogs are not always available immediately. If the catalog does not show up after publishing, navigate to Extensions → Manage Repositories and manually refresh the repository to force a re-sync.

## Contributing

When contributing to this extension:

1. **Follow Standard Patterns**: Use the established domain model and store patterns
2. **Component Organization**: Place components in appropriate directories (formatters/, validators/, pages/)
3. **Type Safety**: Maintain strict TypeScript usage, avoid `any` types
4. **Internationalization**: Add translation keys to l10n/en-us.json for new UI text
5. **Code Quality**: Run `yarn lint` and ensure all pre-commit hooks pass
6. **Feature Flags**: Use feature flags for new functionality
7. **Manual Testing**: Ensure all functionality works across multi-cluster scenarios

### Commit Message Format

This project uses conventional commits enforced by commitlint:

```
type: subject

body (optional)

footer (optional)
```

**Valid types:** `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`, `wip`, `deps`, `security`

Example:
```bash
git commit -m "feat: add multi-cluster installation support"
git commit -m "fix: resolve app installation error handling"
```

## Troubleshooting

### Common Issues

1. **Extension not loading**: Verify URL in developer tools console
2. **Build errors**: Check Node.js version compatibility (requires 20+)
3. **API errors**: Verify cluster permissions and connectivity
4. **Linting errors**: Run `cd pkg/suse-ai-lifecycle-manager && yarn lint` to see details

### Debug Mode

Enable debug logging in development:

```bash
NODE_ENV=development yarn build-pkg suse-ai-lifecycle-manager
```
