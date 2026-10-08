"""Builds the tiny Iris classifiers used by the Android sample app and the demo.

    python3 -m venv .venv && .venv/bin/pip install 'tensorflow==2.17.*' 'numpy<2' scikit-learn
    .venv/bin/python models/iris/make_models.py

Writes iris_v1 (fewer training epochs) and iris_v2 next to this file, each as
TensorFlow Lite (.tflite, input float32[1, 4], output float32[1, 3]) and as a
Core ML neural network (.mlmodel, input "measurements" float32[4], output
"probabilities" float32[3]) built from the same trained weights. Inputs are
sepal length, sepal width, petal length, petal width (cm); outputs are the
probabilities of setosa, versicolor, virginica. Training data: the Iris
dataset (R. A. Fisher, 1936) as shipped with scikit-learn. Core ML export needs
`pip install 'coremltools==8.*'`.
"""
import os

import numpy as np
import tensorflow as tf
from sklearn.datasets import load_iris

HERE = os.path.dirname(os.path.abspath(__file__))


def build(epochs: int, seed: int):
    tf.keras.utils.set_random_seed(seed)
    x, y = load_iris(return_X_y=True)
    x = x.astype(np.float32)
    norm = tf.keras.layers.Normalization()
    norm.adapt(x)
    model = tf.keras.Sequential([
        tf.keras.Input(shape=(4,), batch_size=1, name="measurements"),
        norm,
        tf.keras.layers.Dense(8, activation="relu"),
        tf.keras.layers.Dense(3, activation="softmax"),
    ])
    model.compile(optimizer=tf.keras.optimizers.Adam(0.05), loss="sparse_categorical_crossentropy", metrics=["accuracy"])
    model.fit(x, y, epochs=epochs, batch_size=16, verbose=0)
    _, acc = model.evaluate(x, y, verbose=0)
    print(f"epochs={epochs} training accuracy={acc:.3f}")
    return model, norm


def to_coreml(model, norm, path: str) -> None:
    import coremltools as ct
    from coremltools.models import datatypes
    from coremltools.models.neural_network import NeuralNetworkBuilder

    mean = norm.mean.numpy().reshape(-1)
    std = np.sqrt(norm.variance.numpy().reshape(-1))
    (w1, b1), (w2, b2) = model.layers[1].get_weights(), model.layers[2].get_weights()
    # Fold the normalization into the first dense layer: ((x - mean) / std) @ w1 + b1.
    w1f = w1 / std[:, None]
    b1f = b1 - (mean / std) @ w1
    builder = NeuralNetworkBuilder([("measurements", datatypes.Array(4))], [("probabilities", datatypes.Array(3))])
    builder.add_inner_product("dense1", W=w1f.T, b=b1f, input_channels=4, output_channels=8, has_bias=True,
                              input_name="measurements", output_name="hidden_pre")
    builder.add_activation("relu", "RELU", "hidden_pre", "hidden")
    builder.add_inner_product("dense2", W=w2.T, b=b2, input_channels=8, output_channels=3, has_bias=True,
                              input_name="hidden", output_name="logits")
    builder.add_softmax("softmax", "logits", "probabilities")
    spec = builder.spec
    spec.specificationVersion = 5  # iOS 14+
    spec.description.metadata.license = "Apache-2.0"
    spec.description.metadata.shortDescription = "Iris classifier (FleetML sample)"
    ct.models.MLModel(spec).save(path)


for name, epochs, seed in [("iris_v1", 3, 1), ("iris_v2", 60, 2)]:
    model, norm = build(epochs, seed)
    with open(os.path.join(HERE, name + ".tflite"), "wb") as f:
        f.write(tf.lite.TFLiteConverter.from_keras_model(model).convert())
    to_coreml(model, norm, os.path.join(HERE, name + ".mlmodel"))
