from setuptools import setup, find_packages


def _parse_requirements(path: str) -> list[str]:
    with open(path) as f:
        return [line.strip() for line in f if line.strip() and not line.startswith("#")]


setup(
    name="visualeyes_sre",
    version="1.0.0",
    description="VisualEyes AI-SRE Engine CrewAI-powered Kubernetes Root Cause Analysis",
    author="VisualEyes",
    packages=find_packages(),
    include_package_data=True,
    package_data={
        "": ["*.yaml", "*.yml", "runbooks/**/*.yaml"],
    },
    install_requires=_parse_requirements("requirements.txt"),
    extras_require={
        "dev": _parse_requirements("requirements-dev.txt"),
    },
    entry_points={
        "console_scripts": [
            "veye-ai=cli:cli",
        ],
    },
    python_requires=">=3.11",
    classifiers=[
        "Development Status :: 4 - Beta",
        "Environment :: Console",
        "Intended Audience :: System Administrators",
        "Topic :: System :: Monitoring",
        "Programming Language :: Python :: 3.11",
    ],
)
