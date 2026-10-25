# fleetml

The Python client for [FleetML](https://github.com/fleetml/fleetml), the
self-hosted platform that rolls models out to Android, iOS and Linux devices
with canaries, automatic rollback, A/B experiments and drift detection.

From a training pipeline, a notebook or CI: push a model, start a rollout,
wait for it, and read what the fleet reports. It speaks the server's REST API,
with the same words as the `fleetml` CLI, and depends only on `httpx`.

```bash
pip install fleetml
```

Python 3.9 or later. The package is typed (`py.typed`; its own code passes
`mypy --strict`).

```python
import time

from fleetml import FleetML

fm = FleetML("http://localhost:18080", api_key="...")  # or FLEETML_SERVER, FLEETML_API_KEY
project = fm.projects.create(f"demo-{time.strftime('%Y%m%d-%H%M%S')}")  # names are unique per server
version = fm.push("model.tflite", project=project.id, model="iris", version="1.0.0")
deployment = fm.deployments.start(fm.deployments.create(project.id, version.id).id)
print(fm.wait_for_deployment(deployment.id).status)
```

Documentation: [docs/PYTHON_CLIENT.md](https://github.com/fleetml/fleetml/blob/main/docs/PYTHON_CLIENT.md).
License: Apache-2.0.

## Contact

- Anything about FleetML: [hello@fleetml.dev](mailto:hello@fleetml.dev).
- A security problem: [security@fleetml.dev](mailto:security@fleetml.dev), see [SECURITY.md](https://github.com/fleetml/fleetml/blob/main/SECURITY.md).
- Questions in public: [GitHub Discussions](https://github.com/fleetml/fleetml/discussions).
- Bugs: [GitHub Issues](https://github.com/fleetml/fleetml/issues/new/choose), with the FleetML version and the command or log line that shows the problem.
