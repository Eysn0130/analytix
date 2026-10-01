'use strict'

class Lazy {
  constructor (creator) {
    this.creator = creator
    this.current = undefined
    this.initialized = false
  }

  get hasValue () {
    return this.initialized
  }

  get value () {
    if (!this.initialized) {
      const created = this.creator()
      this.current = created
      this.initialized = true
    }
    return this.current
  }

  set value (next) {
    this.current = next
    this.initialized = true
  }
}

module.exports = { Lazy }
