# SUSE AI Rancher Extension

A Rancher UI Extension for managing SUSE AI applications across Kubernetes clusters. This extension provides a unified interface for installing, managing, and monitoring AI workloads in Rancher-managed clusters.

## Development

### Prerequisites

- Node.js 20+ and Yarn
- Access to a Rancher cluster
- Extension developer features enabled in Rancher

### Setup

1. **Clone and install dependencies:**
   ```bash
   git clone <repository-url>
   cd suse-ai-rancher-ext
   yarn install
   ```

2. **Build the extension:**
   ```bash
   yarn build-pkg suseai-rancher-ext
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

## Manual Testing

The extension provides functionality for:

- **Apps Management**: Browse and install AI applications
- **Multi-cluster Operations**: Install apps across multiple clusters
- **Lifecycle Management**: Upgrade, configure, and uninstall applications
- **Status Monitoring**: Real-time status tracking and error reporting

## Building for Production

```bash
yarn build-pkg suseai-rancher-ext --mode production
```

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
4. **Linting errors**: Run `cd pkg/suseai-rancher-ext && yarn lint` to see details

### Debug Mode

Enable debug logging in development:

```bash
NODE_ENV=development yarn build-pkg suseai-rancher-ext
```
