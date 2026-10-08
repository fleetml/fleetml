# Iris sample models

Tiny classifiers (4 inputs, 8 hidden units, 3 outputs) used by the sample apps
and the demo. v2 is trained for longer than v1. Each version comes in two
formats built from the same trained weights, with identical outputs:

| File | Runtime | Input | Output |
|---|---|---|---|
| `iris_v1.tflite`, `iris_v2.tflite` (about 2.5 KB) | TensorFlow Lite (Android) | `float32[1, 4]` | `float32[1, 3]` |
| `iris_v1.mlmodel`, `iris_v2.mlmodel` (about 0.5 KB) | Core ML (iOS 14+) | `measurements`: `float32[4]` | `probabilities`: `float32[3]` |

Inputs are sepal length, sepal width, petal length and petal width (cm);
outputs are the probabilities of setosa, versicolor and virginica. The Android
sample app bundles `iris_v1.tflite` and the iOS sample app bundles
`iris_v1.mlmodel`, so each can run inference before the server assigns a model.

Rebuild with `python make_models.py` (needs `tensorflow==2.17.*`,
`scikit-learn` and, for the Core ML files, `coremltools==8.*`).

**License.** The models were created by the FleetML project with
[`make_models.py`](make_models.py) and are released under the Apache License 2.0,
like the rest of this repository. They were trained on the Iris dataset
(R. A. Fisher, 1936) as distributed with scikit-learn (BSD-3-Clause); the UCI
Machine Learning Repository publishes the dataset under CC BY 4.0.
