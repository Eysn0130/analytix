#define _DARWIN_C_SOURCE 1

#include <CommonCrypto/CommonDigest.h>
#include <errno.h>
#include <fcntl.h>
#include <libproc.h>
#include <limits.h>
#include <mach/vm_prot.h>
#include <poll.h>
#include <signal.h>
#include <spawn.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/proc_info.h>
#include <sys/proc.h>
#include <sys/stat.h>
#include <sys/syscall.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <time.h>
#include <unistd.h>

/*
 * This build-only adapter deliberately uses the stable N-API v1 C ABI
 * without including headers from the user-owned Node installation. The only
 * production Agent Core remains packages/runtime-go; this file owns the one
 * Darwin primitive Node cannot express: verify the kernel-loaded executable
 * before allowing a path-selected child to run.
 */
typedef struct napi_env__ *napi_env;
typedef struct napi_value__ *napi_value;
typedef struct napi_callback_info__ *napi_callback_info;
typedef struct napi_ref__ *napi_ref;
typedef napi_value (*napi_callback)(napi_env env, napi_callback_info info);
typedef void (*napi_finalize)(napi_env env, void *data, void *hint);

typedef enum {
  napi_undefined = 0,
  napi_null = 1,
  napi_boolean = 2,
  napi_number = 3,
  napi_string = 4,
  napi_symbol = 5,
  napi_object = 6,
  napi_function = 7,
  napi_external = 8,
  napi_bigint = 9
} napi_valuetype;

typedef enum { napi_ok = 0 } napi_status;

extern napi_status napi_get_cb_info(napi_env, napi_callback_info, size_t *,
                                    napi_value *, napi_value *, void **);
extern napi_status napi_typeof(napi_env, napi_value, napi_valuetype *);
extern napi_status napi_get_property_names(napi_env, napi_value, napi_value *);
extern napi_status napi_get_array_length(napi_env, napi_value, uint32_t *);
extern napi_status napi_get_element(napi_env, napi_value, uint32_t, napi_value *);
extern napi_status napi_get_named_property(napi_env, napi_value, const char *,
                                           napi_value *);
extern napi_status napi_get_value_int32(napi_env, napi_value, int32_t *);
extern napi_status napi_get_value_double(napi_env, napi_value, double *);
extern napi_status napi_get_value_string_utf8(napi_env, napi_value, char *,
                                              size_t, size_t *);
extern napi_status napi_is_array(napi_env, napi_value, bool *);
extern napi_status napi_is_buffer(napi_env, napi_value, bool *);
extern napi_status napi_get_buffer_info(napi_env, napi_value, void **, size_t *);
extern napi_status napi_create_object(napi_env, napi_value *);
extern napi_status napi_create_function(napi_env, const char *, size_t,
                                        napi_callback, void *, napi_value *);
extern napi_status napi_set_named_property(napi_env, napi_value, const char *,
                                           napi_value);
extern napi_status napi_create_int32(napi_env, int32_t, napi_value *);
extern napi_status napi_create_string_utf8(napi_env, const char *, size_t,
                                           napi_value *);
extern napi_status napi_create_buffer_copy(napi_env, size_t, const void *,
                                           void **, napi_value *);
extern napi_status napi_get_boolean(napi_env, bool, napi_value *);
extern napi_status napi_throw_error(napi_env, const char *, const char *);
extern napi_status napi_wrap(napi_env, napi_value, void *, napi_finalize, void *,
                             napi_ref *);

#define ANALYTIX_GATE_SCHEMA_VERSION 1
#define ANALYTIX_GATE_MAX_EXECUTABLE_BYTES ((off_t)2 * 1024 * 1024 * 1024)
#define ANALYTIX_GATE_MAX_INHERITED_FDS 5
#define ANALYTIX_GATE_DUPLICATE_FD_BASE 96
#define ANALYTIX_GATE_CD_HASH_BYTES 20
#define ANALYTIX_GATE_MAX_CD_HASHES 64
#define ANALYTIX_GATE_MAX_LOAD_COMMAND_BYTES (16U * 1024U * 1024U)
#define ANALYTIX_GATE_MAX_SIGNATURE_BYTES (64U * 1024U * 1024U)
#define ANALYTIX_GATE_MAX_SIGNATURE_BLOBS 1024U
#define ANALYTIX_GATE_MAX_REGIONS 4096U
#define ANALYTIX_GATE_MAX_PROCESS_IDS 1024U
#define ANALYTIX_GATE_CLEANUP_TIMEOUT_MS 5000
#define ANALYTIX_GATE_POLL_SLICE_MS 10
#define ANALYTIX_GATE_CS_OPS_CDHASH 5U

#define ANALYTIX_GATE_MACHO_64_HEADER_SIZE 32U
#define ANALYTIX_GATE_LC_CODE_SIGNATURE 0x1dU
#define ANALYTIX_GATE_SUPERBLOB_MAGIC 0xfade0cc0U
#define ANALYTIX_GATE_CODEDIRECTORY_MAGIC 0xfade0c02U
#define ANALYTIX_GATE_PRIMARY_SLOT 0U
#define ANALYTIX_GATE_ALTERNATE_FIRST 0x1000U
#define ANALYTIX_GATE_ALTERNATE_LAST 0x1005U

typedef struct {
  const char *name;
  const char *argument;
  size_t inherited_count;
  size_t input_limit;
  size_t stdout_limit;
  size_t stderr_limit;
  int timeout_ms;
} analytix_gate_profile;

static const analytix_gate_profile k_profiles[] = {
    {"build_coordinator", "--analytix-native-build-coordinator-v1", 0,
     8U * 1024U, 16U * 1024U, 16U * 1024U, 1800000},
    {"build_probe", NULL, 1, 4U * 1024U, 8U * 1024U, 8U * 1024U,
     16000},
    {"source_snapshot", "--analytix-native-source-snapshot-v1", 4,
     4U * 1024U, 64U * 1024U, 16U * 1024U, 90000},
    {"source_snapshot_discard",
     "--analytix-native-source-snapshot-discard-v1", 2, 4U * 1024U,
     64U * 1024U, 16U * 1024U, 90000},
    {"generation_publish", "--analytix-native-generation-publisher-v1", 5,
     256U * 1024U, 16U * 1024U, 16U * 1024U, 90000},
};

typedef struct {
  dev_t dev;
  ino_t ino;
  mode_t mode;
  uid_t uid;
  gid_t gid;
  nlink_t nlink;
  off_t size;
  struct timespec mtime;
  struct timespec ctime;
  uint32_t flags;
  uint32_t generation;
} analytix_gate_file_identity;

typedef struct {
  pid_t pid;
  pid_t ppid;
  pid_t pgid;
  uid_t uid;
  uint64_t start_sec;
  uint64_t start_usec;
} analytix_gate_process_identity;

typedef struct {
  uint8_t values[ANALYTIX_GATE_MAX_CD_HASHES][ANALYTIX_GATE_CD_HASH_BYTES];
  size_t count;
} analytix_gate_cd_hashes;

typedef struct {
  uint8_t *data;
  size_t length;
  size_t capacity;
} analytix_gate_buffer;

typedef struct {
  const analytix_gate_profile *profile;
  int executable_fd;
  int inherited_fds[ANALYTIX_GATE_MAX_INHERITED_FDS];
  size_t inherited_count;
  const uint8_t *input;
  size_t input_length;
  off_t expected_size;
  uint8_t expected_sha256[CC_SHA256_DIGEST_LENGTH];
} analytix_gate_request;

typedef struct {
  int exit_code;
  int signal_number;
  analytix_gate_buffer stdout_buffer;
  analytix_gate_buffer stderr_buffer;
  uint8_t loaded_cdhash[ANALYTIX_GATE_CD_HASH_BYTES];
  bool outer_process_reaped;
} analytix_gate_result;

typedef struct {
  bool poisoned;
  bool in_flight;
  bool reentrant_detected;
} analytix_gate_function_state;

typedef enum {
  ANALYTIX_GATE_OUTCOME_CLEAN_REJECTED = 0,
  ANALYTIX_GATE_OUTCOME_VERIFIED = 1,
  ANALYTIX_GATE_OUTCOME_INDETERMINATE_CLEANUP = 2,
  ANALYTIX_GATE_OUTCOME_INDETERMINATE_AFTER_RESUME = 3
} analytix_gate_outcome;

static void analytix_gate_secure_zero(void *value, size_t length) {
  volatile uint8_t *cursor = (volatile uint8_t *)value;
  while (cursor != NULL && length > 0) {
    *cursor++ = 0;
    length--;
  }
}

static bool analytix_gate_constant_time_equal(const uint8_t *left,
                                              const uint8_t *right,
                                              size_t length) {
  uint8_t difference = 0;
  if (left == NULL || right == NULL) {
    return false;
  }
  for (size_t index = 0; index < length; index++) {
    difference |= (uint8_t)(left[index] ^ right[index]);
  }
  return difference == 0;
}

static uint32_t analytix_gate_read_be32(const uint8_t *value) {
  return ((uint32_t)value[0] << 24U) | ((uint32_t)value[1] << 16U) |
         ((uint32_t)value[2] << 8U) | (uint32_t)value[3];
}

static uint64_t analytix_gate_read_be64(const uint8_t *value) {
  return ((uint64_t)analytix_gate_read_be32(value) << 32U) |
         (uint64_t)analytix_gate_read_be32(value + 4);
}

static uint32_t analytix_gate_read_le32(const uint8_t *value) {
  return ((uint32_t)value[3] << 24U) | ((uint32_t)value[2] << 16U) |
         ((uint32_t)value[1] << 8U) | (uint32_t)value[0];
}

static bool analytix_gate_timespec_equal(struct timespec left,
                                         struct timespec right) {
  return left.tv_sec == right.tv_sec && left.tv_nsec == right.tv_nsec;
}

static analytix_gate_file_identity analytix_gate_identity_from_stat(
    const struct stat *value) {
  analytix_gate_file_identity identity;
  memset(&identity, 0, sizeof(identity));
  if (value == NULL) {
    return identity;
  }
  identity.dev = value->st_dev;
  identity.ino = value->st_ino;
  identity.mode = value->st_mode;
  identity.uid = value->st_uid;
  identity.gid = value->st_gid;
  identity.nlink = value->st_nlink;
  identity.size = value->st_size;
  identity.mtime = value->st_mtimespec;
  identity.ctime = value->st_ctimespec;
  identity.flags = value->st_flags;
  identity.generation = value->st_gen;
  return identity;
}

static bool analytix_gate_same_file_identity(
    analytix_gate_file_identity left, analytix_gate_file_identity right) {
  return left.dev == right.dev && left.ino == right.ino &&
         left.mode == right.mode && left.uid == right.uid &&
         left.gid == right.gid && left.nlink == right.nlink &&
         left.size == right.size &&
         analytix_gate_timespec_equal(left.mtime, right.mtime) &&
         analytix_gate_timespec_equal(left.ctime, right.ctime) &&
         left.flags == right.flags && left.generation == right.generation;
}

static bool analytix_gate_valid_executable_identity(
    analytix_gate_file_identity identity, off_t expected_size) {
  mode_t permissions = identity.mode & 07777;
  return S_ISREG(identity.mode) && identity.nlink == 1 &&
         identity.size == expected_size && identity.size > 0 &&
         identity.size <= ANALYTIX_GATE_MAX_EXECUTABLE_BYTES &&
         (permissions == 0700 || permissions == 0500) &&
         (identity.uid == 0 || identity.uid == geteuid());
}

static bool analytix_gate_read_exact_at(int fd, uint8_t *buffer, size_t length,
                                        off_t offset) {
  size_t consumed = 0;
  if (fd < 0 || buffer == NULL || length == 0 || offset < 0) {
    return false;
  }
  while (consumed < length) {
    ssize_t count = pread(fd, buffer + consumed, length - consumed,
                          offset + (off_t)consumed);
    if (count < 0 && errno == EINTR) {
      continue;
    }
    if (count <= 0 || (size_t)count > length - consumed) {
      return false;
    }
    consumed += (size_t)count;
  }
  return true;
}

static bool analytix_gate_sha256_fd(
    int fd, off_t size, uint8_t output[CC_SHA256_DIGEST_LENGTH]) {
  uint8_t *buffer = NULL;
  CC_SHA256_CTX context;
  off_t offset = 0;
  bool valid = false;
  if (fd < 0 || size <= 0 || size > ANALYTIX_GATE_MAX_EXECUTABLE_BYTES ||
      output == NULL) {
    return false;
  }
  buffer = (uint8_t *)malloc(1024U * 1024U);
  if (buffer == NULL) {
    return false;
  }
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
  if (CC_SHA256_Init(&context) != 1) {
    goto cleanup;
  }
  while (offset < size) {
    size_t wanted = (size_t)((size - offset) > (off_t)(1024U * 1024U)
                                 ? (1024U * 1024U)
                                 : (size - offset));
    if (!analytix_gate_read_exact_at(fd, buffer, wanted, offset) ||
        CC_SHA256_Update(&context, buffer, (CC_LONG)wanted) != 1) {
      goto cleanup;
    }
    offset += (off_t)wanted;
  }
  {
    uint8_t extra = 0;
    ssize_t count = pread(fd, &extra, 1, size);
    if (count < 0 && errno == EINTR) {
      do {
        count = pread(fd, &extra, 1, size);
      } while (count < 0 && errno == EINTR);
    }
    if (count != 0 || CC_SHA256_Final(output, &context) != 1) {
      goto cleanup;
    }
  }
  valid = true;
cleanup:
#pragma clang diagnostic pop
  analytix_gate_secure_zero(&context, sizeof(context));
  analytix_gate_secure_zero(buffer, 1024U * 1024U);
  free(buffer);
  return valid;
}

static bool analytix_gate_parse_lower_hex(const char *value, size_t length,
                                          uint8_t *output,
                                          size_t output_length) {
  if (value == NULL || output == NULL || length != output_length * 2U) {
    return false;
  }
  for (size_t index = 0; index < output_length; index++) {
    uint8_t byte = 0;
    for (size_t nibble = 0; nibble < 2; nibble++) {
      char character = value[index * 2U + nibble];
      uint8_t digit;
      if (character >= '0' && character <= '9') {
        digit = (uint8_t)(character - '0');
      } else if (character >= 'a' && character <= 'f') {
        digit = (uint8_t)(character - 'a' + 10);
      } else {
        return false;
      }
      byte = (uint8_t)((byte << 4U) | digit);
    }
    output[index] = byte;
  }
  return true;
}

static void analytix_gate_encode_lower_hex(const uint8_t *value, size_t length,
                                           char *output) {
  static const char alphabet[] = "0123456789abcdef";
  for (size_t index = 0; index < length; index++) {
    output[index * 2U] = alphabet[value[index] >> 4U];
    output[index * 2U + 1U] = alphabet[value[index] & 0x0fU];
  }
  output[length * 2U] = '\0';
}

static bool analytix_gate_add_cdhash(analytix_gate_cd_hashes *hashes,
                                     const uint8_t *value) {
  if (hashes == NULL || value == NULL) {
    return false;
  }
  for (size_t index = 0; index < hashes->count; index++) {
    if (analytix_gate_constant_time_equal(hashes->values[index], value,
                                          ANALYTIX_GATE_CD_HASH_BYTES)) {
      return true;
    }
  }
  if (hashes->count >= ANALYTIX_GATE_MAX_CD_HASHES) {
    return false;
  }
  memcpy(hashes->values[hashes->count], value,
         ANALYTIX_GATE_CD_HASH_BYTES);
  hashes->count++;
  return true;
}

static bool analytix_gate_hash_code_directory(
    const uint8_t *blob, size_t length,
    uint8_t output[ANALYTIX_GATE_CD_HASH_BYTES]) {
  if (blob == NULL || output == NULL || length < 40U) {
    return false;
  }
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
  switch (blob[37]) {
  case 1: {
    uint8_t digest[CC_SHA1_DIGEST_LENGTH];
    if (CC_SHA1(blob, (CC_LONG)length, digest) == NULL) {
      return false;
    }
    memcpy(output, digest, ANALYTIX_GATE_CD_HASH_BYTES);
    analytix_gate_secure_zero(digest, sizeof(digest));
    break;
  }
  case 2:
  case 3: {
    uint8_t digest[CC_SHA256_DIGEST_LENGTH];
    if (CC_SHA256(blob, (CC_LONG)length, digest) == NULL) {
      return false;
    }
    memcpy(output, digest, ANALYTIX_GATE_CD_HASH_BYTES);
    analytix_gate_secure_zero(digest, sizeof(digest));
    break;
  }
  case 4: {
    uint8_t digest[CC_SHA384_DIGEST_LENGTH];
    if (CC_SHA384(blob, (CC_LONG)length, digest) == NULL) {
      return false;
    }
    memcpy(output, digest, ANALYTIX_GATE_CD_HASH_BYTES);
    analytix_gate_secure_zero(digest, sizeof(digest));
    break;
  }
  default:
    return false;
  }
#pragma clang diagnostic pop
  return true;
}

static bool analytix_gate_hash_superblob(const uint8_t *signature,
                                         size_t signature_length,
                                         analytix_gate_cd_hashes *hashes) {
  uint32_t length;
  uint32_t count;
  if (signature == NULL || hashes == NULL || signature_length < 12U ||
      analytix_gate_read_be32(signature) != ANALYTIX_GATE_SUPERBLOB_MAGIC) {
    return false;
  }
  length = analytix_gate_read_be32(signature + 4);
  count = analytix_gate_read_be32(signature + 8);
  if (length < 12U || length > signature_length || count == 0 ||
      count > ANALYTIX_GATE_MAX_SIGNATURE_BLOBS ||
      (uint64_t)12U + (uint64_t)count * 8U > length) {
    return false;
  }
  for (uint32_t index = 0; index < count; index++) {
    size_t entry = 12U + (size_t)index * 8U;
    uint32_t slot = analytix_gate_read_be32(signature + entry);
    uint32_t offset = analytix_gate_read_be32(signature + entry + 4U);
    uint32_t blob_length;
    uint8_t cdhash[ANALYTIX_GATE_CD_HASH_BYTES];
    if (slot != ANALYTIX_GATE_PRIMARY_SLOT &&
        (slot < ANALYTIX_GATE_ALTERNATE_FIRST ||
         slot > ANALYTIX_GATE_ALTERNATE_LAST)) {
      continue;
    }
    if (offset > length - 8U ||
        analytix_gate_read_be32(signature + offset) !=
            ANALYTIX_GATE_CODEDIRECTORY_MAGIC) {
      return false;
    }
    blob_length = analytix_gate_read_be32(signature + offset + 4U);
    if (blob_length < 40U || blob_length > length - offset ||
        !analytix_gate_hash_code_directory(signature + offset, blob_length,
                                           cdhash) ||
        !analytix_gate_add_cdhash(hashes, cdhash)) {
      analytix_gate_secure_zero(cdhash, sizeof(cdhash));
      return false;
    }
    analytix_gate_secure_zero(cdhash, sizeof(cdhash));
  }
  return hashes->count > 0;
}

static bool analytix_gate_macho_slice_cdhashes(
    int fd, uint64_t base, uint64_t slice_size,
    analytix_gate_cd_hashes *hashes) {
  uint8_t header[ANALYTIX_GATE_MACHO_64_HEADER_SIZE];
  uint32_t command_count;
  uint32_t command_bytes;
  uint32_t signature_offset = 0;
  uint32_t signature_size = 0;
  uint8_t *commands = NULL;
  uint8_t *signature = NULL;
  bool little_endian;
  bool valid = false;
  if (fd < 0 || hashes == NULL ||
      slice_size < ANALYTIX_GATE_MACHO_64_HEADER_SIZE || base > INT64_MAX ||
      !analytix_gate_read_exact_at(fd, header, sizeof(header), (off_t)base)) {
    return false;
  }
  if (analytix_gate_read_le32(header) == 0xfeedfacfU) {
    little_endian = true;
  } else if (analytix_gate_read_be32(header) == 0xfeedfacfU) {
    little_endian = false;
  } else {
    return false;
  }
  command_count = little_endian ? analytix_gate_read_le32(header + 16)
                                : analytix_gate_read_be32(header + 16);
  command_bytes = little_endian ? analytix_gate_read_le32(header + 20)
                                : analytix_gate_read_be32(header + 20);
  if (command_count == 0 || command_count > 4096U || command_bytes < 8U ||
      command_bytes > ANALYTIX_GATE_MAX_LOAD_COMMAND_BYTES ||
      (uint64_t)ANALYTIX_GATE_MACHO_64_HEADER_SIZE + command_bytes >
          slice_size) {
    return false;
  }
  commands = (uint8_t *)malloc(command_bytes);
  if (commands == NULL ||
      !analytix_gate_read_exact_at(
          fd, commands, command_bytes,
          (off_t)(base + ANALYTIX_GATE_MACHO_64_HEADER_SIZE))) {
    goto cleanup;
  }
  {
    uint32_t cursor = 0;
    for (uint32_t index = 0; index < command_count; index++) {
      uint32_t command;
      uint32_t size;
      if (cursor > command_bytes - 8U) {
        goto cleanup;
      }
      command = little_endian ? analytix_gate_read_le32(commands + cursor)
                              : analytix_gate_read_be32(commands + cursor);
      size = little_endian ? analytix_gate_read_le32(commands + cursor + 4U)
                           : analytix_gate_read_be32(commands + cursor + 4U);
      if (size < 8U || size % 4U != 0 || size > command_bytes - cursor) {
        goto cleanup;
      }
      if (command == ANALYTIX_GATE_LC_CODE_SIGNATURE) {
        if (size != 16U || signature_size != 0) {
          goto cleanup;
        }
        signature_offset =
            little_endian
                ? analytix_gate_read_le32(commands + cursor + 8U)
                : analytix_gate_read_be32(commands + cursor + 8U);
        signature_size =
            little_endian
                ? analytix_gate_read_le32(commands + cursor + 12U)
                : analytix_gate_read_be32(commands + cursor + 12U);
      }
      cursor += size;
    }
    if (cursor != command_bytes) {
      goto cleanup;
    }
  }
  if (signature_size < 12U ||
      signature_size > ANALYTIX_GATE_MAX_SIGNATURE_BYTES ||
      (uint64_t)signature_offset + signature_size > slice_size ||
      base + signature_offset > INT64_MAX) {
    goto cleanup;
  }
  signature = (uint8_t *)malloc(signature_size);
  if (signature == NULL ||
      !analytix_gate_read_exact_at(fd, signature, signature_size,
                                   (off_t)(base + signature_offset)) ||
      !analytix_gate_hash_superblob(signature, signature_size, hashes)) {
    goto cleanup;
  }
  valid = true;
cleanup:
  if (signature != NULL) {
    analytix_gate_secure_zero(signature, signature_size);
    free(signature);
  }
  if (commands != NULL) {
    analytix_gate_secure_zero(commands, command_bytes);
    free(commands);
  }
  return valid;
}

static bool analytix_gate_opened_cdhashes(int fd, off_t file_size,
                                          analytix_gate_cd_hashes *hashes) {
  uint8_t prefix[8];
  uint32_t magic;
  uint32_t count;
  bool fat64;
  size_t entry_size;
  size_t table_size;
  uint8_t *table = NULL;
  bool valid = false;
  if (fd < 0 || file_size < (off_t)ANALYTIX_GATE_MACHO_64_HEADER_SIZE ||
      hashes == NULL) {
    return false;
  }
  memset(hashes, 0, sizeof(*hashes));
  if (!analytix_gate_read_exact_at(fd, prefix, sizeof(prefix), 0)) {
    return false;
  }
  magic = analytix_gate_read_be32(prefix);
  if (magic != 0xcafebabeU && magic != 0xcafebabfU) {
    return analytix_gate_macho_slice_cdhashes(fd, 0, (uint64_t)file_size,
                                              hashes);
  }
  fat64 = magic == 0xcafebabfU;
  count = analytix_gate_read_be32(prefix + 4);
  entry_size = fat64 ? 32U : 20U;
  if (count == 0 || count > 32U ||
      (size_t)count > (SIZE_MAX - 8U) / entry_size) {
    return false;
  }
  table_size = (size_t)count * entry_size;
  if ((uint64_t)8U + table_size > (uint64_t)file_size) {
    return false;
  }
  table = (uint8_t *)malloc(table_size);
  if (table == NULL ||
      !analytix_gate_read_exact_at(fd, table, table_size, 8)) {
    goto cleanup;
  }
  for (uint32_t index = 0; index < count; index++) {
    const uint8_t *entry = table + (size_t)index * entry_size;
    uint64_t offset = fat64 ? analytix_gate_read_be64(entry + 8U)
                            : analytix_gate_read_be32(entry + 8U);
    uint64_t size = fat64 ? analytix_gate_read_be64(entry + 16U)
                          : analytix_gate_read_be32(entry + 12U);
    if (size < ANALYTIX_GATE_MACHO_64_HEADER_SIZE ||
        offset > (uint64_t)file_size || size > (uint64_t)file_size - offset ||
        !analytix_gate_macho_slice_cdhashes(fd, offset, size, hashes)) {
      goto cleanup;
    }
  }
  valid = hashes->count > 0;
cleanup:
  if (table != NULL) {
    analytix_gate_secure_zero(table, table_size);
    free(table);
  }
  return valid;
}

static bool analytix_gate_cdhash_member(
    const analytix_gate_cd_hashes *hashes,
    const uint8_t value[ANALYTIX_GATE_CD_HASH_BYTES]) {
  if (hashes == NULL || value == NULL) {
    return false;
  }
  for (size_t index = 0; index < hashes->count; index++) {
    if (analytix_gate_constant_time_equal(hashes->values[index], value,
                                          ANALYTIX_GATE_CD_HASH_BYTES)) {
      return true;
    }
  }
  return false;
}

static int analytix_gate_duplicate_fd(int source, int minimum) {
  int duplicated;
  if (source < 0 || minimum < 3) {
    return -1;
  }
  do {
    duplicated = fcntl(source, F_DUPFD_CLOEXEC, minimum);
  } while (duplicated < 0 && errno == EINTR);
  return duplicated;
}

static bool analytix_gate_set_nonblocking(int fd) {
  int flags;
  if (fd < 0) {
    return false;
  }
  do {
    flags = fcntl(fd, F_GETFL);
  } while (flags < 0 && errno == EINTR);
  if (flags < 0) {
    return false;
  }
  while (fcntl(fd, F_SETFL, flags | O_NONBLOCK) < 0) {
    if (errno != EINTR) {
      return false;
    }
  }
  return true;
}

static bool analytix_gate_pipe_cloexec(int descriptors[2]) {
  if (descriptors == NULL || pipe(descriptors) != 0) {
    return false;
  }
  for (size_t index = 0; index < 2; index++) {
    int flags;
    do {
      flags = fcntl(descriptors[index], F_GETFD);
    } while (flags < 0 && errno == EINTR);
    if (flags < 0) {
      return false;
    }
    while (fcntl(descriptors[index], F_SETFD, flags | FD_CLOEXEC) < 0) {
      if (errno != EINTR) {
        return false;
      }
    }
  }
  return true;
}

static int64_t analytix_gate_monotonic_milliseconds(void) {
  struct timespec value;
  if (clock_gettime(CLOCK_MONOTONIC, &value) != 0 || value.tv_sec < 0 ||
      value.tv_nsec < 0 || value.tv_nsec >= 1000000000L ||
      value.tv_sec > INT64_MAX / 1000) {
    return -1;
  }
  return (int64_t)value.tv_sec * 1000 + value.tv_nsec / 1000000;
}

static bool analytix_gate_read_process_identity(
    pid_t pid, analytix_gate_process_identity *out, uint32_t *status) {
  struct proc_bsdinfo info;
  int count;
  if (pid <= 0 || out == NULL || status == NULL) {
    return false;
  }
  memset(&info, 0, sizeof(info));
  count = proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &info, sizeof(info));
  if (count != (int)sizeof(info) || info.pbi_pid != (uint32_t)pid ||
      info.pbi_ppid > INT32_MAX || info.pbi_pgid == 0 ||
      info.pbi_pgid > INT32_MAX || info.pbi_uid != geteuid() ||
      info.pbi_status == 0 || info.pbi_status > SZOMB ||
      info.pbi_start_tvsec == 0 || info.pbi_start_tvusec >= 1000000U) {
    return false;
  }
  out->pid = pid;
  out->ppid = (pid_t)info.pbi_ppid;
  out->pgid = (pid_t)info.pbi_pgid;
  out->uid = info.pbi_uid;
  out->start_sec = info.pbi_start_tvsec;
  out->start_usec = info.pbi_start_tvusec;
  *status = info.pbi_status;
  return true;
}

static bool analytix_gate_same_process_identity(
    analytix_gate_process_identity left,
    analytix_gate_process_identity right) {
  return left.pid == right.pid && left.ppid == right.ppid &&
         left.pgid == right.pgid && left.uid == right.uid &&
         left.start_sec == right.start_sec &&
         left.start_usec == right.start_usec;
}

static bool analytix_gate_process_ids(int type, uint32_t type_info,
                                      pid_t *values, size_t capacity,
                                      size_t *count) {
  int bytes;
  if (values == NULL || capacity == 0 || count == NULL ||
      capacity > INT_MAX / sizeof(pid_t)) {
    return false;
  }
  memset(values, 0, capacity * sizeof(pid_t));
  do {
    bytes = proc_listpids((uint32_t)type, type_info, values,
                         (int)(capacity * sizeof(pid_t)));
  } while (bytes < 0 && errno == EINTR);
  if (bytes < 0 || (size_t)bytes > capacity * sizeof(pid_t) ||
      bytes % (int)sizeof(pid_t) != 0 ||
      (size_t)bytes == capacity * sizeof(pid_t)) {
    return false;
  }
  *count = (size_t)bytes / sizeof(pid_t);
  for (size_t index = 0; index < *count; index++) {
    if (values[index] <= 0) {
      return false;
    }
  }
  return true;
}

static bool analytix_gate_no_initial_children(
    analytix_gate_process_identity expected) {
  pid_t children[ANALYTIX_GATE_MAX_PROCESS_IDS];
  size_t count = 0;
  analytix_gate_process_identity current;
  uint32_t status = 0;
  for (size_t pass = 0; pass < 2; pass++) {
    if (!analytix_gate_read_process_identity(expected.pid, &current, &status) ||
        !analytix_gate_same_process_identity(expected, current) ||
        status != SSTOP ||
        !analytix_gate_process_ids(PROC_PPID_ONLY,
                                   (uint32_t)expected.pid, children,
                                   ANALYTIX_GATE_MAX_PROCESS_IDS, &count) ||
        count != 0) {
      return false;
    }
  }
  return true;
}

static bool analytix_gate_loaded_region_matches(
    pid_t pid, analytix_gate_file_identity expected) {
  uint64_t address = 0;
  if (pid <= 0) {
    return false;
  }
  for (size_t index = 0; index < ANALYTIX_GATE_MAX_REGIONS; index++) {
    struct proc_regionwithpathinfo region;
    const struct proc_regioninfo *info;
    const struct vinfo_stat *stat;
    int count;
    memset(&region, 0, sizeof(region));
    count = proc_pidinfo(pid, PROC_PIDREGIONPATHINFO, address, &region,
                         sizeof(region));
    if (count == 0) {
      break;
    }
    if (count != (int)sizeof(region)) {
      return false;
    }
    info = &region.prp_prinfo;
    stat = &region.prp_vip.vip_vi.vi_stat;
    if (info->pri_size == 0 || info->pri_address < address ||
        info->pri_address > UINT64_MAX - info->pri_size) {
      return false;
    }
    if ((info->pri_protection & VM_PROT_EXECUTE) != 0 && stat->vst_ino != 0) {
      /* The lowest file-backed executable region is the Mach-O main image. */
      return (uint32_t)expected.dev == stat->vst_dev &&
             (uint64_t)expected.ino == stat->vst_ino &&
             (uint16_t)expected.mode == stat->vst_mode &&
             (uint16_t)expected.nlink == stat->vst_nlink &&
             expected.uid == stat->vst_uid && expected.gid == stat->vst_gid &&
             expected.size == stat->vst_size &&
             expected.mtime.tv_sec == stat->vst_mtime &&
             expected.mtime.tv_nsec == stat->vst_mtimensec &&
             expected.ctime.tv_sec == stat->vst_ctime &&
             expected.ctime.tv_nsec == stat->vst_ctimensec;
    }
    address = info->pri_address + info->pri_size;
  }
  return false;
}

static bool analytix_gate_loaded_cdhash(
    pid_t pid, const analytix_gate_cd_hashes *expected,
    uint8_t output[ANALYTIX_GATE_CD_HASH_BYTES]) {
  long result;
  if (pid <= 0 || expected == NULL || output == NULL) {
    return false;
  }
  memset(output, 0, ANALYTIX_GATE_CD_HASH_BYTES);
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
  do {
    result = syscall(SYS_csops, pid, ANALYTIX_GATE_CS_OPS_CDHASH, output,
                     ANALYTIX_GATE_CD_HASH_BYTES);
  } while (result < 0 && errno == EINTR);
#pragma clang diagnostic pop
  return result == 0 && analytix_gate_cdhash_member(expected, output);
}

static bool analytix_gate_waitid_observe(pid_t pid, bool *exited) {
  siginfo_t info;
  int result;
  if (pid <= 0 || exited == NULL) {
    return false;
  }
  memset(&info, 0, sizeof(info));
  do {
    result = waitid(P_PID, (id_t)pid, &info, WEXITED | WNOHANG | WNOWAIT);
  } while (result < 0 && errno == EINTR);
  if (result != 0) {
    return false;
  }
  *exited = info.si_pid == pid;
  return info.si_pid == 0 || info.si_pid == pid;
}

static bool analytix_gate_group_only_contains_leader(pid_t pid) {
  pid_t values[ANALYTIX_GATE_MAX_PROCESS_IDS];
  size_t count = 0;
  bool leader_seen = false;
  if (pid <= 0 ||
      !analytix_gate_process_ids(PROC_PGRP_ONLY, (uint32_t)pid, values,
                                 ANALYTIX_GATE_MAX_PROCESS_IDS, &count)) {
    return false;
  }
  for (size_t index = 0; index < count; index++) {
    if (values[index] == pid && !leader_seen) {
      leader_seen = true;
      continue;
    }
    return false;
  }
  return leader_seen;
}

static bool analytix_gate_group_empty(pid_t pid) {
  pid_t values[ANALYTIX_GATE_MAX_PROCESS_IDS];
  size_t count = 0;
  return pid > 0 &&
         analytix_gate_process_ids(PROC_PGRP_ONLY, (uint32_t)pid, values,
                                   ANALYTIX_GATE_MAX_PROCESS_IDS, &count) &&
         count == 0;
}

static bool analytix_gate_kill_and_wait(pid_t pid, int *wait_status,
                                        bool require_group_proof) {
  int64_t start = analytix_gate_monotonic_milliseconds();
  int status = 0;
  pid_t waited;
  if (pid <= 0 || wait_status == NULL || start < 0) {
    return false;
  }
  if (kill(-pid, SIGKILL) != 0 && errno != ESRCH) {
    /* Continue with the exact direct child kill; cleanup is still ambiguous. */
    require_group_proof = true;
  }
  if (kill(pid, SIGKILL) != 0 && errno != ESRCH) {
    return false;
  }
  for (;;) {
    bool observed = false;
    if (!analytix_gate_waitid_observe(pid, &observed)) {
      return false;
    }
    if (observed) {
      break;
    }
    if (analytix_gate_monotonic_milliseconds() - start >=
        ANALYTIX_GATE_CLEANUP_TIMEOUT_MS) {
      return false;
    }
    usleep(1000);
  }
  if (require_group_proof) {
    int64_t deadline = start + ANALYTIX_GATE_CLEANUP_TIMEOUT_MS;
    while (!analytix_gate_group_only_contains_leader(pid)) {
      if (analytix_gate_monotonic_milliseconds() >= deadline) {
        return false;
      }
      if (kill(-pid, SIGKILL) != 0 && errno != ESRCH) {
        return false;
      }
      usleep(1000);
    }
  }
  do {
    waited = waitpid(pid, &status, 0);
  } while (waited < 0 && errno == EINTR);
  if (waited != pid) {
    return false;
  }
  if (require_group_proof) {
    int64_t deadline = analytix_gate_monotonic_milliseconds() +
                       ANALYTIX_GATE_CLEANUP_TIMEOUT_MS;
    while (!analytix_gate_group_empty(pid)) {
      if (analytix_gate_monotonic_milliseconds() >= deadline) {
        return false;
      }
      usleep(1000);
    }
  }
  *wait_status = status;
  return true;
}

static bool analytix_gate_close_fd_once(int *fd) {
  int value;
  if (fd == NULL || *fd < 0) {
    return true;
  }
  value = *fd;
  *fd = -1;
  return close(value) == 0;
}

static bool analytix_gate_buffer_init(analytix_gate_buffer *buffer,
                                      size_t limit) {
  if (buffer == NULL || limit == 0 || limit == SIZE_MAX) {
    return false;
  }
  memset(buffer, 0, sizeof(*buffer));
  buffer->data = (uint8_t *)malloc(limit + 1U);
  if (buffer->data == NULL) {
    return false;
  }
  buffer->capacity = limit + 1U;
  return true;
}

static void analytix_gate_buffer_destroy(analytix_gate_buffer *buffer) {
  if (buffer == NULL) {
    return;
  }
  if (buffer->data != NULL) {
    analytix_gate_secure_zero(buffer->data, buffer->capacity);
    free(buffer->data);
  }
  memset(buffer, 0, sizeof(*buffer));
}

static bool analytix_gate_read_pipe(int *fd, analytix_gate_buffer *buffer,
                                    size_t limit, bool *closed) {
  uint8_t chunk[4096];
  if (fd == NULL || buffer == NULL || closed == NULL || *fd < 0 ||
      buffer->data == NULL || buffer->capacity != limit + 1U) {
    return false;
  }
  for (;;) {
    ssize_t count = read(*fd, chunk, sizeof(chunk));
    if (count > 0) {
      if ((size_t)count > buffer->capacity - buffer->length) {
        return false;
      }
      memcpy(buffer->data + buffer->length, chunk, (size_t)count);
      buffer->length += (size_t)count;
      if (buffer->length > limit) {
        return false;
      }
      continue;
    }
    if (count == 0) {
      if (!analytix_gate_close_fd_once(fd)) {
        return false;
      }
      *closed = true;
      return true;
    }
    if (errno == EINTR) {
      continue;
    }
    if (errno == EAGAIN || errno == EWOULDBLOCK) {
      return true;
    }
    return false;
  }
}

static bool analytix_gate_drive_child(
    pid_t pid, const analytix_gate_request *request, int *stdin_fd,
    int *stdout_fd, int *stderr_fd, analytix_gate_result *result,
    int64_t operation_deadline) {
  size_t input_offset = 0;
  bool stdout_closed = false;
  bool stderr_closed = false;
  bool exited = false;
  if (pid <= 0 || request == NULL || stdin_fd == NULL || stdout_fd == NULL ||
      stderr_fd == NULL || result == NULL || operation_deadline <= 0 ||
      !analytix_gate_set_nonblocking(*stdin_fd) ||
      !analytix_gate_set_nonblocking(*stdout_fd) ||
      !analytix_gate_set_nonblocking(*stderr_fd)) {
    return false;
  }
  while (!exited) {
    struct pollfd polls[3];
    nfds_t poll_count = 0;
    int64_t now = analytix_gate_monotonic_milliseconds();
    int timeout;
    int poll_result;
    if (now < 0 || now >= operation_deadline) {
      return false;
    }
    if (*stdin_fd >= 0) {
      if (input_offset == request->input_length) {
        if (!analytix_gate_close_fd_once(stdin_fd)) {
          return false;
        }
      } else {
        polls[poll_count++] =
            (struct pollfd){.fd = *stdin_fd, .events = POLLOUT, .revents = 0};
      }
    }
    if (*stdout_fd >= 0) {
      polls[poll_count++] = (struct pollfd){.fd = *stdout_fd,
                                           .events = POLLIN | POLLHUP,
                                           .revents = 0};
    }
    if (*stderr_fd >= 0) {
      polls[poll_count++] = (struct pollfd){.fd = *stderr_fd,
                                           .events = POLLIN | POLLHUP,
                                           .revents = 0};
    }
    timeout = (int)(operation_deadline - now);
    if (timeout > ANALYTIX_GATE_POLL_SLICE_MS) {
      timeout = ANALYTIX_GATE_POLL_SLICE_MS;
    }
    do {
      poll_result = poll(polls, poll_count, timeout);
    } while (poll_result < 0 && errno == EINTR);
    if (poll_result < 0) {
      return false;
    }
    for (nfds_t index = 0; index < poll_count; index++) {
      short events = polls[index].revents;
      if ((events & (POLLERR | POLLNVAL)) != 0) {
        return false;
      }
      if (polls[index].fd == *stdin_fd &&
          (events & (POLLOUT | POLLHUP)) != 0) {
        ssize_t written = write(*stdin_fd, request->input + input_offset,
                                request->input_length - input_offset);
        if (written > 0 && (size_t)written <=
                               request->input_length - input_offset) {
          input_offset += (size_t)written;
          if (input_offset == request->input_length) {
            if (!analytix_gate_close_fd_once(stdin_fd)) {
              return false;
            }
          }
        } else if (written < 0 && errno != EINTR && errno != EAGAIN &&
                   errno != EWOULDBLOCK) {
          return false;
        }
      } else if (polls[index].fd == *stdout_fd &&
                 (events & (POLLIN | POLLHUP)) != 0) {
        if (!analytix_gate_read_pipe(stdout_fd, &result->stdout_buffer,
                                     request->profile->stdout_limit,
                                     &stdout_closed)) {
          return false;
        }
      } else if (polls[index].fd == *stderr_fd &&
                 (events & (POLLIN | POLLHUP)) != 0) {
        if (!analytix_gate_read_pipe(stderr_fd, &result->stderr_buffer,
                                     request->profile->stderr_limit,
                                     &stderr_closed)) {
          return false;
        }
      }
    }
    if (!analytix_gate_waitid_observe(pid, &exited)) {
      return false;
    }
  }
  return analytix_gate_close_fd_once(stdin_fd);
}

static bool analytix_gate_drain_after_exit(
    int *stdout_fd, int *stderr_fd, const analytix_gate_request *request,
    analytix_gate_result *result) {
  int64_t deadline = analytix_gate_monotonic_milliseconds() +
                     ANALYTIX_GATE_CLEANUP_TIMEOUT_MS;
  bool stdout_closed = *stdout_fd < 0;
  bool stderr_closed = *stderr_fd < 0;
  if (request == NULL || result == NULL || deadline <= 0) {
    return false;
  }
  while (!stdout_closed || !stderr_closed) {
    struct pollfd polls[2];
    nfds_t count = 0;
    int64_t now = analytix_gate_monotonic_milliseconds();
    int wait_ms;
    int poll_result;
    if (now < 0 || now >= deadline) {
      return false;
    }
    if (*stdout_fd >= 0) {
      polls[count++] = (struct pollfd){.fd = *stdout_fd,
                                      .events = POLLIN | POLLHUP,
                                      .revents = 0};
    }
    if (*stderr_fd >= 0) {
      polls[count++] = (struct pollfd){.fd = *stderr_fd,
                                      .events = POLLIN | POLLHUP,
                                      .revents = 0};
    }
    wait_ms = (int)(deadline - now);
    if (wait_ms > ANALYTIX_GATE_POLL_SLICE_MS) {
      wait_ms = ANALYTIX_GATE_POLL_SLICE_MS;
    }
    do {
      poll_result = poll(polls, count, wait_ms);
    } while (poll_result < 0 && errno == EINTR);
    if (poll_result < 0) {
      return false;
    }
    for (nfds_t index = 0; index < count; index++) {
      if ((polls[index].revents & (POLLERR | POLLNVAL)) != 0) {
        return false;
      }
      if (polls[index].fd == *stdout_fd &&
          (polls[index].revents & (POLLIN | POLLHUP)) != 0) {
        if (!analytix_gate_read_pipe(stdout_fd, &result->stdout_buffer,
                                     request->profile->stdout_limit,
                                     &stdout_closed)) {
          return false;
        }
      } else if (polls[index].fd == *stderr_fd &&
                 (polls[index].revents & (POLLIN | POLLHUP)) != 0) {
        if (!analytix_gate_read_pipe(stderr_fd, &result->stderr_buffer,
                                     request->profile->stderr_limit,
                                     &stderr_closed)) {
          return false;
        }
      }
    }
  }
  return true;
}

static analytix_gate_outcome analytix_gate_spawn_and_run(
    const analytix_gate_request *request, analytix_gate_result *result) {
  int executable_fd = -1;
  int inherited[ANALYTIX_GATE_MAX_INHERITED_FDS] = {-1, -1, -1, -1, -1};
  int stdin_pipe[2] = {-1, -1};
  int stdout_pipe[2] = {-1, -1};
  int stderr_pipe[2] = {-1, -1};
  int root_fd = -1;
  posix_spawnattr_t attributes;
  posix_spawn_file_actions_t actions;
  bool attributes_initialized = false;
  bool actions_initialized = false;
  struct stat executable_stat;
  analytix_gate_file_identity executable_identity;
  analytix_gate_cd_hashes expected_cdhashes;
  uint8_t executable_digest[CC_SHA256_DIGEST_LENGTH];
  char executable_path[PATH_MAX];
  char *arguments[3] = {NULL, NULL, NULL};
  char *environment[] = {"LANG=C", "LC_ALL=C", "TZ=UTC", NULL};
  pid_t pid = 0;
  bool spawned = false;
  bool success = false;
  bool resume_attempted = false;
  bool cleanup_ok = true;
  int wait_status = 0;
  int64_t started_at;
  int64_t deadline;
  analytix_gate_process_identity first_process;
  analytix_gate_process_identity second_process;
  uint32_t first_status = 0;
  uint32_t second_status = 0;

  memset(&executable_stat, 0, sizeof(executable_stat));
  memset(&expected_cdhashes, 0, sizeof(expected_cdhashes));
  memset(executable_digest, 0, sizeof(executable_digest));
  memset(executable_path, 0, sizeof(executable_path));
  memset(&first_process, 0, sizeof(first_process));
  memset(&second_process, 0, sizeof(second_process));

  if (request == NULL || result == NULL || request->profile == NULL ||
      request->inherited_count != request->profile->inherited_count) {
    return ANALYTIX_GATE_OUTCOME_CLEAN_REJECTED;
  }
  if (!analytix_gate_buffer_init(&result->stdout_buffer,
                                 request->profile->stdout_limit) ||
      !analytix_gate_buffer_init(&result->stderr_buffer,
                                 request->profile->stderr_limit)) {
    goto cleanup;
  }
  executable_fd = analytix_gate_duplicate_fd(
      request->executable_fd, ANALYTIX_GATE_DUPLICATE_FD_BASE);
  if (executable_fd < ANALYTIX_GATE_DUPLICATE_FD_BASE) {
    goto cleanup;
  }
  for (size_t index = 0; index < request->inherited_count; index++) {
    inherited[index] = analytix_gate_duplicate_fd(
        request->inherited_fds[index],
        ANALYTIX_GATE_DUPLICATE_FD_BASE + 1 + (int)index);
    if (inherited[index] < ANALYTIX_GATE_DUPLICATE_FD_BASE) {
      goto cleanup;
    }
  }
  if (fstat(executable_fd, &executable_stat) != 0) {
    goto cleanup;
  }
  executable_identity = analytix_gate_identity_from_stat(&executable_stat);
  if (!analytix_gate_valid_executable_identity(executable_identity,
                                               request->expected_size) ||
      !analytix_gate_sha256_fd(executable_fd, executable_identity.size,
                               executable_digest) ||
      !analytix_gate_constant_time_equal(executable_digest,
                                          request->expected_sha256,
                                          sizeof(executable_digest)) ||
      !analytix_gate_opened_cdhashes(executable_fd, executable_identity.size,
                                     &expected_cdhashes) ||
      fcntl(executable_fd, F_GETPATH, executable_path) != 0 ||
      executable_path[0] != '/' ||
      memchr(executable_path, '\0', sizeof(executable_path)) == NULL) {
    goto cleanup;
  }
  {
    struct stat path_stat;
    if (lstat(executable_path, &path_stat) != 0 ||
        S_ISLNK(path_stat.st_mode) ||
        !analytix_gate_same_file_identity(
            executable_identity,
            analytix_gate_identity_from_stat(&path_stat))) {
      goto cleanup;
    }
  }
  if (!analytix_gate_pipe_cloexec(stdin_pipe) ||
      !analytix_gate_pipe_cloexec(stdout_pipe) ||
      !analytix_gate_pipe_cloexec(stderr_pipe)) {
    goto cleanup;
  }
  root_fd = open("/", O_RDONLY | O_DIRECTORY | O_CLOEXEC | O_NOFOLLOW);
  if (root_fd < 0 || posix_spawnattr_init(&attributes) != 0) {
    goto cleanup;
  }
  attributes_initialized = true;
  {
    short flags = POSIX_SPAWN_START_SUSPENDED | POSIX_SPAWN_SETSID |
                  POSIX_SPAWN_CLOEXEC_DEFAULT;
    if (posix_spawnattr_setflags(&attributes, flags) != 0 ||
        posix_spawn_file_actions_init(&actions) != 0) {
      goto cleanup;
    }
  }
  actions_initialized = true;
  if (posix_spawn_file_actions_adddup2(&actions, stdin_pipe[0], 0) != 0 ||
      posix_spawn_file_actions_adddup2(&actions, stdout_pipe[1], 1) != 0 ||
      posix_spawn_file_actions_adddup2(&actions, stderr_pipe[1], 2) != 0) {
    goto cleanup;
  }
  for (size_t index = 0; index < request->inherited_count; index++) {
    if (posix_spawn_file_actions_adddup2(&actions, inherited[index],
                                         3 + (int)index) != 0) {
      goto cleanup;
    }
  }
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
  if (posix_spawn_file_actions_addfchdir_np(&actions, root_fd) != 0) {
    goto cleanup;
  }
#pragma clang diagnostic pop
  arguments[0] = executable_path;
  if (request->profile->argument != NULL) {
    arguments[1] = (char *)request->profile->argument;
  }
  started_at = analytix_gate_monotonic_milliseconds();
  if (started_at < 0 ||
      posix_spawn(&pid, executable_path, &actions, &attributes, arguments,
                  environment) != 0 ||
      pid <= 0) {
    pid = 0;
    goto cleanup;
  }
  spawned = true;
  deadline = started_at + request->profile->timeout_ms;
  if (!analytix_gate_close_fd_once(&stdin_pipe[0]) ||
      !analytix_gate_close_fd_once(&stdout_pipe[1]) ||
      !analytix_gate_close_fd_once(&stderr_pipe[1])) {
    cleanup_ok = false;
    goto cleanup;
  }

  if (!analytix_gate_read_process_identity(pid, &first_process, &first_status) ||
      first_process.ppid != getpid() || first_process.pgid != pid ||
      first_process.uid != geteuid() || first_status != SSTOP ||
      !analytix_gate_no_initial_children(first_process) ||
      !analytix_gate_loaded_region_matches(pid, executable_identity) ||
      !analytix_gate_loaded_cdhash(pid, &expected_cdhashes,
                                   result->loaded_cdhash) ||
      fstat(executable_fd, &executable_stat) != 0 ||
      !analytix_gate_same_file_identity(
          executable_identity,
          analytix_gate_identity_from_stat(&executable_stat)) ||
      !analytix_gate_sha256_fd(executable_fd, executable_identity.size,
                               executable_digest) ||
      !analytix_gate_constant_time_equal(executable_digest,
                                          request->expected_sha256,
                                          sizeof(executable_digest)) ||
      !analytix_gate_read_process_identity(pid, &second_process, &second_status) ||
      !analytix_gate_same_process_identity(first_process, second_process) ||
      second_status != SSTOP) {
    goto cleanup;
  }
  resume_attempted = true;
  if (kill(pid, SIGCONT) != 0) {
    goto cleanup;
  }
  if (!analytix_gate_drive_child(pid, request, &stdin_pipe[1],
                                 &stdout_pipe[0], &stderr_pipe[0], result,
                                 deadline) ||
      !analytix_gate_kill_and_wait(pid, &wait_status, true)) {
    goto cleanup;
  }
  spawned = false;
  result->outer_process_reaped = true;
  if (!analytix_gate_drain_after_exit(&stdout_pipe[0], &stderr_pipe[0],
                                      request, result) ||
      fstat(executable_fd, &executable_stat) != 0 ||
      !analytix_gate_same_file_identity(
          executable_identity,
          analytix_gate_identity_from_stat(&executable_stat)) ||
      !analytix_gate_sha256_fd(executable_fd, executable_identity.size,
                               executable_digest) ||
      !analytix_gate_constant_time_equal(executable_digest,
                                          request->expected_sha256,
                                          sizeof(executable_digest))) {
    goto cleanup;
  }
  if (WIFEXITED(wait_status)) {
    result->exit_code = WEXITSTATUS(wait_status);
    result->signal_number = 0;
  } else if (WIFSIGNALED(wait_status)) {
    result->exit_code = -1;
    result->signal_number = WTERMSIG(wait_status);
  } else {
    goto cleanup;
  }
  success = true;

cleanup:
  if (spawned) {
    int ignored_status = 0;
    if (!analytix_gate_kill_and_wait(pid, &ignored_status, true)) {
      cleanup_ok = false;
    }
  }
  if (actions_initialized) {
    if (posix_spawn_file_actions_destroy(&actions) != 0) {
      success = false;
      cleanup_ok = false;
    }
  }
  if (attributes_initialized) {
    if (posix_spawnattr_destroy(&attributes) != 0) {
      success = false;
      cleanup_ok = false;
    }
  }
  if (!analytix_gate_close_fd_once(&root_fd)) {
    cleanup_ok = false;
  }
  if (!analytix_gate_close_fd_once(&stdin_pipe[0])) {
    cleanup_ok = false;
  }
  if (!analytix_gate_close_fd_once(&stdin_pipe[1])) {
    cleanup_ok = false;
  }
  if (!analytix_gate_close_fd_once(&stdout_pipe[0])) {
    cleanup_ok = false;
  }
  if (!analytix_gate_close_fd_once(&stdout_pipe[1])) {
    cleanup_ok = false;
  }
  if (!analytix_gate_close_fd_once(&stderr_pipe[0])) {
    cleanup_ok = false;
  }
  if (!analytix_gate_close_fd_once(&stderr_pipe[1])) {
    cleanup_ok = false;
  }
  if (!analytix_gate_close_fd_once(&executable_fd)) {
    cleanup_ok = false;
  }
  for (size_t index = 0; index < ANALYTIX_GATE_MAX_INHERITED_FDS;
       index++) {
    if (!analytix_gate_close_fd_once(&inherited[index])) {
      cleanup_ok = false;
    }
  }
  analytix_gate_secure_zero(executable_digest, sizeof(executable_digest));
  analytix_gate_secure_zero(&expected_cdhashes, sizeof(expected_cdhashes));
  if (!success || !cleanup_ok) {
    analytix_gate_buffer_destroy(&result->stdout_buffer);
    analytix_gate_buffer_destroy(&result->stderr_buffer);
    analytix_gate_secure_zero(result->loaded_cdhash,
                              sizeof(result->loaded_cdhash));
    result->outer_process_reaped = false;
  }
  if (success && cleanup_ok) {
    return ANALYTIX_GATE_OUTCOME_VERIFIED;
  }
  if (resume_attempted) {
    return ANALYTIX_GATE_OUTCOME_INDETERMINATE_AFTER_RESUME;
  }
  if (!cleanup_ok) {
    return ANALYTIX_GATE_OUTCOME_INDETERMINATE_CLEANUP;
  }
  return ANALYTIX_GATE_OUTCOME_CLEAN_REJECTED;
}

static const analytix_gate_profile *analytix_gate_find_profile(
    const char *name) {
  if (name == NULL) {
    return NULL;
  }
  for (size_t index = 0; index < sizeof(k_profiles) / sizeof(k_profiles[0]);
       index++) {
    if (strcmp(name, k_profiles[index].name) == 0) {
      return &k_profiles[index];
    }
  }
  return NULL;
}

static bool analytix_gate_napi_type(napi_env env, napi_value value,
                                    napi_valuetype expected) {
  napi_valuetype actual = napi_undefined;
  return value != NULL && napi_typeof(env, value, &actual) == napi_ok &&
         actual == expected;
}

static bool analytix_gate_napi_string(napi_env env, napi_value value,
                                      char *output, size_t capacity,
                                      size_t *length) {
  size_t measured = 0;
  if (output == NULL || capacity == 0 || length == NULL ||
      !analytix_gate_napi_type(env, value, napi_string) ||
      napi_get_value_string_utf8(env, value, NULL, 0, &measured) != napi_ok ||
      measured >= capacity ||
      napi_get_value_string_utf8(env, value, output, capacity, length) !=
          napi_ok ||
      *length != measured || output[measured] != '\0') {
    return false;
  }
  return true;
}

static bool analytix_gate_parse_request(napi_env env, napi_value arguments[7],
                                        size_t count,
                                        analytix_gate_request *request) {
  int32_t schema_version = 0;
  int32_t executable_fd = -1;
  double executable_size = 0;
  char profile_name[32];
  size_t profile_length = 0;
  char sha256[65];
  size_t sha256_length = 0;
  bool is_buffer = false;
  bool is_array = false;
  void *input = NULL;
  size_t input_length = 0;
  uint32_t inherited_count = 0;
  if (arguments == NULL || request == NULL) {
    return false;
  }
  memset(request, 0, sizeof(*request));
  request->executable_fd = -1;
  for (size_t index = 0; index < ANALYTIX_GATE_MAX_INHERITED_FDS;
       index++) {
    request->inherited_fds[index] = -1;
  }
  if (count != 7 ||
      !analytix_gate_napi_type(env, arguments[0], napi_number) ||
      napi_get_value_int32(env, arguments[0], &schema_version) != napi_ok ||
      schema_version != ANALYTIX_GATE_SCHEMA_VERSION ||
      !analytix_gate_napi_string(env, arguments[1], profile_name,
                                 sizeof(profile_name), &profile_length) ||
      profile_length == 0 ||
      (request->profile = analytix_gate_find_profile(profile_name)) == NULL ||
      !analytix_gate_napi_type(env, arguments[2], napi_number) ||
      napi_get_value_int32(env, arguments[2], &executable_fd) != napi_ok ||
      executable_fd < 0 ||
      !analytix_gate_napi_string(env, arguments[3], sha256, sizeof(sha256),
                                 &sha256_length) ||
      sha256_length != 64 ||
      !analytix_gate_parse_lower_hex(sha256, sha256_length,
                                     request->expected_sha256,
                                     sizeof(request->expected_sha256)) ||
      !analytix_gate_napi_type(env, arguments[4], napi_number) ||
      napi_get_value_double(env, arguments[4], &executable_size) != napi_ok ||
      executable_size < 1 ||
      executable_size > (double)ANALYTIX_GATE_MAX_EXECUTABLE_BYTES ||
      (double)(off_t)executable_size != executable_size ||
      napi_is_buffer(env, arguments[5], &is_buffer) != napi_ok || !is_buffer ||
      napi_get_buffer_info(env, arguments[5], &input, &input_length) != napi_ok ||
      input == NULL || input_length == 0 ||
      input_length > request->profile->input_limit ||
      napi_is_array(env, arguments[6], &is_array) != napi_ok || !is_array ||
      napi_get_array_length(env, arguments[6], &inherited_count) != napi_ok ||
      inherited_count != request->profile->inherited_count ||
      inherited_count > ANALYTIX_GATE_MAX_INHERITED_FDS) {
    analytix_gate_secure_zero(sha256, sizeof(sha256));
    return false;
  }
  request->executable_fd = executable_fd;
  request->expected_size = (off_t)executable_size;
  request->input = (const uint8_t *)input;
  request->input_length = input_length;
  request->inherited_count = inherited_count;
  for (uint32_t index = 0; index < inherited_count; index++) {
    napi_value element = NULL;
    int32_t fd = -1;
    if (napi_get_element(env, arguments[6], index, &element) != napi_ok ||
        !analytix_gate_napi_type(env, element, napi_number) ||
        napi_get_value_int32(env, element, &fd) != napi_ok || fd < 0 ||
        fd == executable_fd) {
      analytix_gate_secure_zero(sha256, sizeof(sha256));
      return false;
    }
    for (uint32_t prior = 0; prior < index; prior++) {
      if (request->inherited_fds[prior] == fd) {
        analytix_gate_secure_zero(sha256, sizeof(sha256));
        return false;
      }
    }
    request->inherited_fds[index] = fd;
  }
  analytix_gate_secure_zero(sha256, sizeof(sha256));
  return true;
}

static bool analytix_gate_set_result_property(napi_env env, napi_value object,
                                              const char *name,
                                              napi_value value) {
  return env != NULL && object != NULL && name != NULL && value != NULL &&
         napi_set_named_property(env, object, name, value) == napi_ok;
}

static napi_value analytix_gate_throw(napi_env env, const char *code,
                                      const char *message) {
  (void)napi_throw_error(env, code, message);
  return NULL;
}

static napi_value analytix_gate_invoke(napi_env env, napi_callback_info info) {
  analytix_gate_request request;
  analytix_gate_result result;
  analytix_gate_function_state *state = NULL;
  analytix_gate_outcome outcome;
  napi_value arguments[7];
  size_t count = sizeof(arguments) / sizeof(arguments[0]);
  void *callback_data = NULL;
  napi_value object = NULL;
  napi_value value = NULL;
  char cdhash_hex[ANALYTIX_GATE_CD_HASH_BYTES * 2U + 1U];
  memset(&request, 0, sizeof(request));
  memset(&result, 0, sizeof(result));
  memset(arguments, 0, sizeof(arguments));
  memset(cdhash_hex, 0, sizeof(cdhash_hex));
  result.exit_code = -1;
  if (napi_get_cb_info(env, info, &count, arguments, NULL, &callback_data) !=
          napi_ok ||
      callback_data == NULL) {
    return analytix_gate_throw(
        env, "ANALYTIX_DARWIN_AUTHORITY_GATE_INVALID_REQUEST",
        "Darwin authority launch gate request is invalid");
  }
  state = (analytix_gate_function_state *)callback_data;
  if (state->poisoned) {
    return analytix_gate_throw(
        env, "ANALYTIX_DARWIN_AUTHORITY_GATE_POISONED",
        "Darwin authority launch gate is poisoned");
  }
  if (state->in_flight) {
    state->reentrant_detected = true;
    return analytix_gate_throw(
        env, "ANALYTIX_DARWIN_AUTHORITY_GATE_REENTRANT",
        "Darwin authority launch gate rejected reentrant execution");
  }
  state->in_flight = true;
  state->reentrant_detected = false;
  if (!analytix_gate_parse_request(env, arguments, count, &request) ||
      state->reentrant_detected) {
    if (state->reentrant_detected) {
      state->poisoned = true;
    }
    state->in_flight = false;
    analytix_gate_secure_zero(&request, sizeof(request));
    return analytix_gate_throw(
        env,
        state->reentrant_detected
            ? "ANALYTIX_DARWIN_AUTHORITY_GATE_REENTRANT"
            : "ANALYTIX_DARWIN_AUTHORITY_GATE_INVALID_REQUEST",
        state->reentrant_detected
            ? "Darwin authority launch gate rejected reentrant execution"
            : "Darwin authority launch gate request is invalid");
  }
  outcome = analytix_gate_spawn_and_run(&request, &result);
  if (outcome != ANALYTIX_GATE_OUTCOME_VERIFIED) {
    const char *code = "ANALYTIX_DARWIN_AUTHORITY_GATE_REJECTED";
    const char *message = "Darwin authority launch gate rejected execution";
    if (outcome == ANALYTIX_GATE_OUTCOME_INDETERMINATE_CLEANUP) {
      state->poisoned = true;
      code = "ANALYTIX_DARWIN_AUTHORITY_GATE_INDETERMINATE_CLEANUP";
      message = "Darwin authority launch gate cleanup is indeterminate";
    } else if (outcome ==
               ANALYTIX_GATE_OUTCOME_INDETERMINATE_AFTER_RESUME) {
      state->poisoned = true;
      code = "ANALYTIX_DARWIN_AUTHORITY_GATE_INDETERMINATE_AFTER_RESUME";
      message = "Darwin authority launch gate execution is indeterminate";
    }
    state->in_flight = false;
    analytix_gate_secure_zero(&request, sizeof(request));
    return analytix_gate_throw(env, code, message);
  }
  analytix_gate_encode_lower_hex(result.loaded_cdhash,
                                 sizeof(result.loaded_cdhash), cdhash_hex);
  if (napi_create_object(env, &object) != napi_ok ||
      napi_create_int32(env, result.exit_code, &value) != napi_ok ||
      !analytix_gate_set_result_property(env, object, "exitCode", value) ||
      napi_create_int32(env, result.signal_number, &value) != napi_ok ||
      !analytix_gate_set_result_property(env, object, "signal", value) ||
      napi_create_buffer_copy(env, result.stdout_buffer.length,
                              result.stdout_buffer.data, NULL, &value) !=
          napi_ok ||
      !analytix_gate_set_result_property(env, object, "stdout", value) ||
      napi_create_buffer_copy(env, result.stderr_buffer.length,
                              result.stderr_buffer.data, NULL, &value) !=
          napi_ok ||
      !analytix_gate_set_result_property(env, object, "stderr", value) ||
      napi_create_string_utf8(env, cdhash_hex,
                              ANALYTIX_GATE_CD_HASH_BYTES * 2U, &value) !=
          napi_ok ||
      !analytix_gate_set_result_property(env, object, "loadedCdhash", value) ||
      napi_get_boolean(env, result.outer_process_reaped, &value) != napi_ok ||
      !analytix_gate_set_result_property(env, object, "outerProcessReaped",
                                         value)) {
    state->poisoned = true;
    state->in_flight = false;
    analytix_gate_buffer_destroy(&result.stdout_buffer);
    analytix_gate_buffer_destroy(&result.stderr_buffer);
    analytix_gate_secure_zero(&request, sizeof(request));
    analytix_gate_secure_zero(&result, sizeof(result));
    analytix_gate_secure_zero(cdhash_hex, sizeof(cdhash_hex));
    return analytix_gate_throw(
        env, "ANALYTIX_DARWIN_AUTHORITY_GATE_INDETERMINATE_AFTER_RESUME",
        "Darwin authority launch gate result is indeterminate");
  }
  analytix_gate_buffer_destroy(&result.stdout_buffer);
  analytix_gate_buffer_destroy(&result.stderr_buffer);
  analytix_gate_secure_zero(&request, sizeof(request));
  analytix_gate_secure_zero(&result, sizeof(result));
  analytix_gate_secure_zero(cdhash_hex, sizeof(cdhash_hex));
  state->in_flight = false;
  return object;
}

static void analytix_gate_state_finalize(napi_env env, void *data,
                                         void *hint) {
  analytix_gate_function_state *state =
      (analytix_gate_function_state *)data;
  (void)env;
  (void)hint;
  if (state != NULL) {
    analytix_gate_secure_zero(state, sizeof(*state));
    free(state);
  }
}

__attribute__((visibility("default"))) int32_t
node_api_module_get_api_version_v1(void) {
  return 1;
}

__attribute__((visibility("default"))) napi_value
napi_register_module_v1(napi_env env, napi_value exports) {
  napi_value function = NULL;
  analytix_gate_function_state *state = NULL;
  if (env == NULL || exports == NULL) {
    return NULL;
  }
  state = (analytix_gate_function_state *)calloc(1, sizeof(*state));
  if (state == NULL ||
      napi_create_function(env, "invokePinnedV1", (size_t)-1,
                           analytix_gate_invoke, state, &function) != napi_ok) {
    if (state != NULL) {
      analytix_gate_secure_zero(state, sizeof(*state));
      free(state);
    }
    return NULL;
  }
  if (napi_wrap(env, function, state, analytix_gate_state_finalize, NULL,
                NULL) != napi_ok) {
    analytix_gate_secure_zero(state, sizeof(*state));
    free(state);
    return NULL;
  }
  if (napi_set_named_property(env, exports, "invokePinnedV1", function) !=
      napi_ok) {
    return NULL;
  }
  return exports;
}
