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

```python
from fleetml import FleetML

fm = FleetML("http://localhost:18080", api_key="...")  # or FLEETML_SERVER, FLEETML_API_KEY
project = fm.projects.create("demo")
version = fm.push("model.tflite", project=project.id, model="iris", version="1.0.0")
deployment = fm.deployments.start(fm.deployments.create(project.id, version.id).id)
print(fm.wait_for_deployment(deployment.id).status)
```

Documentation: [docs/PYTHON_CLIENT.md](https://github.com/fleetml/fleetml/blob/main/docs/PYTHON_CLIENT.md).
License: Apache-2.0.
