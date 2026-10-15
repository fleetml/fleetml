"""The error every FleetML call raises."""

from __future__ import annotations


class FleetMLError(Exception):
    """A request the server refused, or one that did not reach it.

    ``status`` is the HTTP status (0 when the server could not be reached),
    ``code`` the server's error code (such as ``NOT_FOUND``, ``CONFLICT``,
    ``VALIDATION_ERROR``, ``INTEGRITY_ERROR``, ``UNAUTHORIZED``) and
    ``message`` the server's message.
    """

    def __init__(self, status: int, code: str, message: str) -> None:
        self.status = status
        self.code = code
        self.message = message
        super().__init__(f"{status} {code}: {message}" if status else f"{code}: {message}")
