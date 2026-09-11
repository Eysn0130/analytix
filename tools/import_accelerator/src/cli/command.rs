use std::path::PathBuf;

#[derive(Debug, PartialEq, Eq)]
pub(crate) enum Command {
    DetectEncoding {
        path: PathBuf,
    },
    PrepareCsv {
        path: PathBuf,
        limit: usize,
        output: Option<PathBuf>,
        encoding: Option<String>,
    },
    PrepareCsvWithProfile {
        path: PathBuf,
        limit: usize,
        encoding: Option<String>,
    },
    BatchPrepareCsvWithProfile {
        manifest: PathBuf,
    },
    BatchPrepareCsv {
        manifest: PathBuf,
    },
    ProfileColumns {
        path: PathBuf,
        encoding: Option<String>,
    },
    Hashes {
        path: PathBuf,
        algos: Vec<String>,
    },
    ExcelToCsv {
        input: PathBuf,
        output: PathBuf,
    },
    SplitAccountSections {
        input: PathBuf,
        output_dir: PathBuf,
    },
}
