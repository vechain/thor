import pytest

MODEL_A = "Qwen/Qwen2.5-1.5B-Instruct"  # honest / requested model
MODEL_B = "Qwen/Qwen2.5-0.5B-Instruct"  # cheap / fraud model


def pytest_configure(config):
    config.addinivalue_line(
        "markers",
        "integration: marks tests requiring model download (~4GB)",
    )


@pytest.fixture(scope="session")
def prover_a():
    from commitllm.prover import CommitLLMProver

    return CommitLLMProver(MODEL_A)


@pytest.fixture(scope="session")
def prover_b():
    from commitllm.prover import CommitLLMProver

    return CommitLLMProver(MODEL_B)


@pytest.fixture(scope="session")
def verifier_a():
    from commitllm.verifier import CommitLLMVerifier

    return CommitLLMVerifier(MODEL_A)
