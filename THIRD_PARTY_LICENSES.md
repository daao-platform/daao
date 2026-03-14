# Third-Party Licenses

This file lists all third-party dependencies used in DAAO and their licenses.

---

## Go Dependencies (Backend — Nexus & Satellite)

### Direct Dependencies

| Package | Version | License |
|---------|---------|---------|
| github.com/golang-jwt/jwt/v5 | v5.2.1 | MIT |
| github.com/golang-migrate/migrate/v4 | v4.19.0 | MIT |
| github.com/google/uuid | v1.6.0 | BSD-3-Clause |
| github.com/gorilla/websocket | v1.5.3 | BSD-2-Clause |
| github.com/jackc/pgx/v5 | v5.7.6 | MIT |
| github.com/lib/pq | v1.10.9 | MIT |
| github.com/quic-go/quic-go | v0.59.0 | MIT |
| github.com/quic-go/webtransport-go | v0.10.0 | MIT |
| github.com/stretchr/testify | v1.11.1 | MIT |
| github.com/testcontainers/testcontainers-go | v0.35.0 | MIT |
| golang.org/x/sys | v0.41.0 | BSD-3-Clause |
| golang.org/x/term | v0.39.0 | BSD-3-Clause |
| google.golang.org/grpc | v1.78.0 | Apache-2.0 |
| google.golang.org/protobuf | v1.36.11 | BSD-3-Clause |
| gopkg.in/yaml.v3 | v3.0.1 | MIT / Apache-2.0 |

### Indirect Dependencies

| Package | Version | License |
|---------|---------|---------|
| dario.cat/mergo | v1.0.0 | BSD-3-Clause |
| github.com/Azure/go-ansiterm | v0.0.0 | MIT |
| github.com/Microsoft/go-winio | v0.6.2 | MIT |
| github.com/cenkalti/backoff/v4 | v4.2.1 | MIT |
| github.com/cespare/xxhash/v2 | v2.3.0 | MIT |
| github.com/containerd/containerd | v1.7.18 | Apache-2.0 |
| github.com/containerd/log | v0.1.0 | Apache-2.0 |
| github.com/containerd/platforms | v0.2.1 | Apache-2.0 |
| github.com/cpuguy83/dockercfg | v0.3.2 | MIT |
| github.com/davecgh/go-spew | v1.1.1 | ISC |
| github.com/distribution/reference | v0.6.0 | Apache-2.0 |
| github.com/docker/docker | v28.3.3 | Apache-2.0 |
| github.com/docker/go-connections | v0.5.0 | Apache-2.0 |
| github.com/docker/go-units | v0.5.0 | Apache-2.0 |
| github.com/dunglas/httpsfv | v1.1.0 | MIT |
| github.com/felixge/httpsnoop | v1.0.4 | MIT |
| github.com/go-logr/logr | v1.4.3 | Apache-2.0 |
| github.com/go-logr/stdr | v1.2.2 | Apache-2.0 |
| github.com/go-ole/go-ole | v1.2.6 | MIT |
| github.com/gogo/protobuf | v1.3.2 | BSD-3-Clause |
| github.com/hashicorp/errwrap | v1.1.0 | MPL-2.0 |
| github.com/hashicorp/go-multierror | v1.1.1 | MPL-2.0 |
| github.com/jackc/pgpassfile | v1.0.0 | MIT |
| github.com/jackc/pgservicefile | v0.0.0 | MIT |
| github.com/jackc/puddle/v2 | v2.2.2 | MIT |
| github.com/klauspost/compress | v1.18.2 | BSD-3-Clause / MIT |
| github.com/lufia/plan9stats | v0.0.0 | MIT |
| github.com/magiconair/properties | v1.8.7 | BSD-2-Clause |
| github.com/moby/docker-image-spec | v1.3.1 | Apache-2.0 |
| github.com/moby/patternmatcher | v0.6.0 | Apache-2.0 |
| github.com/moby/sys/sequential | v0.6.0 | Apache-2.0 |
| github.com/moby/sys/user | v0.4.0 | Apache-2.0 |
| github.com/moby/term | v0.5.0 | Apache-2.0 |
| github.com/morikuni/aec | v1.0.0 | MIT |
| github.com/opencontainers/go-digest | v1.0.0 | Apache-2.0 |
| github.com/opencontainers/image-spec | v1.1.0 | Apache-2.0 |
| github.com/pkg/errors | v0.9.1 | BSD-2-Clause |
| github.com/pmezard/go-difflib | v1.0.0 | BSD-3-Clause |
| github.com/power-devops/perfstat | v0.0.0 | MIT |
| github.com/quic-go/qpack | v0.6.0 | MIT |
| github.com/robfig/cron/v3 | v3.0.1 | MIT |
| github.com/shirou/gopsutil/v3 | v3.23.12 | BSD-3-Clause |
| github.com/shoenig/go-m1cpu | v0.1.6 | MPL-2.0 |
| github.com/sirupsen/logrus | v1.9.3 | MIT |
| github.com/tklauser/go-sysconf | v0.3.12 | BSD-3-Clause |
| github.com/tklauser/numcpus | v0.6.1 | Apache-2.0 |
| github.com/yusufpapurcu/wmi | v1.2.3 | MIT |
| go.opentelemetry.io/* | v1.40.0 | Apache-2.0 |
| golang.org/x/crypto | v0.47.0 | BSD-3-Clause |
| golang.org/x/net | v0.49.0 | BSD-3-Clause |
| golang.org/x/sync | v0.19.0 | BSD-3-Clause |
| golang.org/x/text | v0.33.0 | BSD-3-Clause |
| google.golang.org/genproto | v0.0.0 | Apache-2.0 |
| gotest.tools/v3 | v3.5.2 | Apache-2.0 |

---

## Node.js Dependencies (Frontend — Cockpit)

### Production Dependencies

| Package | Version | License |
|---------|---------|---------|
| @monaco-editor/react | ^4.6.0 | MIT |
| @xterm/addon-fit | ^0.10.0 | MIT |
| @xterm/addon-web-links | ^0.11.0 | MIT |
| @xterm/xterm | ^5.5.0 | MIT |
| monaco-editor | ^0.52.0 | MIT |
| react | ^19.0.0 | MIT |
| react-dom | ^19.0.0 | MIT |
| react-router-dom | ^7.13.1 | MIT |

### Development Dependencies

| Package | Version | License |
|---------|---------|---------|
| @eslint/js | ^9.0.0 | MIT |
| @testing-library/dom | ^10.4.1 | MIT |
| @testing-library/react | ^16.3.2 | MIT |
| @types/react | ^19.0.0 | MIT |
| @types/react-dom | ^19.0.0 | MIT |
| @vitejs/plugin-react | ^4.7.0 | MIT |
| eslint | ^9.0.0 | MIT |
| eslint-plugin-react | ^7.37.0 | MIT |
| eslint-plugin-react-hooks | ^5.0.0 | MIT |
| eslint-plugin-react-refresh | ^0.4.0 | MIT |
| jsdom | ^28.1.0 | MIT |
| typescript | ^5.7.2 | Apache-2.0 |
| typescript-eslint | ^8.0.0 | MIT |
| vite | ^6.0.3 | MIT |
| vitest | ^4.0.18 | MIT |

---

## Planned Dependencies

| Package | License | Notes |
|---------|---------|-------|
| Pi SDK (badlogic/pi-mono) | MIT | Agent runtime engine for Agent Forge |

---

## License Summary

| License | Count | Notes |
|---------|-------|-------|
| MIT | ~60 | Most permissive; no restrictions |
| Apache-2.0 | ~20 | Requires attribution in NOTICE file |
| BSD-2-Clause | 3 | Minimal attribution required |
| BSD-3-Clause | ~15 | Standard BSD with non-endorsement clause |
| MPL-2.0 | 3 | File-level copyleft; compatible with BSL |
| ISC | 1 | Functionally equivalent to MIT |

> All licenses are compatible with the Business Source License 1.1 used by DAAO. No GPL or AGPL dependencies exist.
