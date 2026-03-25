## ADDED Requirements

### Requirement: Override binary takes precedence over sidecar of same name
When the widget dispatch layer resolves a widget name, it SHALL check for an `orcai-<name>` binary on PATH before consulting sidecar declarations. If an override binary is found, the sidecar with the same name SHALL NOT be launched. Sidecars whose names have no corresponding override binary are unaffected.

#### Scenario: Override binary shadows same-named sidecar
- **WHEN** a sidecar named `weather` exists in `~/.config/orcai/widgets/weather/widget.yaml` and `orcai-weather` is found on PATH
- **THEN** dispatch executes `orcai-weather` and does not launch the sidecar binary declared in the manifest

#### Scenario: Sidecar launched when no override binary present
- **WHEN** a sidecar named `weather` exists and no `orcai-weather` binary is found on PATH
- **THEN** dispatch launches the sidecar binary declared in the manifest as normal

#### Scenario: Override applies to both core and third-party sidecars
- **WHEN** `orcai-picker` is found on PATH and `picker` is registered as a core subcommand (not a sidecar)
- **THEN** dispatch executes `orcai-picker`, not `orcai picker`
