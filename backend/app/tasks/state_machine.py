from typing import Dict, Set

from app.tasks.models import TaskStatus


class InvalidTaskTransitionError(ValueError):
    pass


_ALLOWED_TRANSITIONS: Dict[TaskStatus, Set[TaskStatus]] = {
    TaskStatus.QUEUED: {TaskStatus.RUNNING, TaskStatus.FAILED, TaskStatus.CANCELED},
    TaskStatus.RUNNING: {TaskStatus.SUCCEEDED, TaskStatus.FAILED, TaskStatus.CANCELED},
    TaskStatus.FAILED: {TaskStatus.QUEUED},
    TaskStatus.CANCELED: {TaskStatus.QUEUED},
    TaskStatus.SUCCEEDED: set(),
}


class TaskStateMachine:
    @staticmethod
    def can_transition(current: TaskStatus, target: TaskStatus) -> bool:
        if current == target:
            return True
        return target in _ALLOWED_TRANSITIONS[current]

    @staticmethod
    def ensure_transition(current: TaskStatus, target: TaskStatus) -> None:
        if not TaskStateMachine.can_transition(current, target):
            raise InvalidTaskTransitionError(
                "Invalid transition: {0} -> {1}".format(current.value, target.value)
            )

    @staticmethod
    def transition_map() -> Dict[str, list]:
        return {
            state.value: [target.value for target in sorted(targets, key=lambda x: x.value)]
            for state, targets in _ALLOWED_TRANSITIONS.items()
        }
