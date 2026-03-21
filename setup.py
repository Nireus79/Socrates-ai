#!/usr/bin/env python
"""Setup script for socrates-ai package."""

from setuptools import setup, find_packages

setup(
    packages=find_packages(include=["socratic_system", "socratic_system.*"]),
)
