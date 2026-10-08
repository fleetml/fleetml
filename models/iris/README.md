# Iris sample models

`iris_v1.tflite` and `iris_v2.tflite` are tiny classifiers (4 inputs, 8 hidden
units, 3 outputs; about 2.5 KB each) used by the Android sample app and the
demo. v2 is trained for longer than v1. The Android sample app bundles
`iris_v1.tflite` as `iris_bundled.tflite` so it can run inference before the
server assigns a model.

- Input: `float32[1, 4]`: sepal length, sepal width, petal length, petal width (cm).
- Output: `float32[1, 3]`: probabilities for setosa, versicolor, virginica.

**License.** The models were created by the FleetML project with
[`make_models.py`](make_models.py) and are released under the Apache License 2.0,
like the rest of this repository. They were trained on the Iris dataset
(R. A. Fisher, 1936) as distributed with scikit-learn (BSD-3-Clause); the UCI
Machine Learning Repository publishes the dataset under CC BY 4.0.
