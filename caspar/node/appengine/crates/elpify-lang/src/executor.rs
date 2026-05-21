use std::fmt::{Display, Formatter};
use std::fs;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{mpsc, Arc, Mutex};
use std::thread;

use miden_vm::{
    prove, verify, AdviceInputs, Assembler, DefaultHost, ExecutionProof, Program, ProgramInfo,
    ProvingOptions, StackInputs, StackOutputs, VerificationError,
};

#[derive(Debug, Clone)]
pub struct ExecutionArtifacts {
    pub stack_outputs: Vec<u64>,
    pub proof_bytes: Vec<u8>,
    pub program_info: ProgramInfo,
    pub stack_inputs: StackInputs,
}

#[derive(Debug)]
pub enum ExecutorError {
    Assembly(String),
    Input(String),
    Execution(String),
    ProofDeserialization(String),
    Verification(String),
}

impl Display for ExecutorError {
    fn fmt(&self, f: &mut Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Assembly(msg) => write!(f, "assembly error: {msg}"),
            Self::Input(msg) => write!(f, "input error: {msg}"),
            Self::Execution(msg) => write!(f, "execution/proving error: {msg}"),
            Self::ProofDeserialization(msg) => write!(f, "proof deserialization error: {msg}"),
            Self::Verification(msg) => write!(f, "verification error: {msg}"),
        }
    }
}

impl std::error::Error for ExecutorError {}

pub fn assemble_program(masm: &str) -> Result<Program, ExecutorError> {
    Assembler::default()
        .assemble_program(masm)
        .map_err(|e| ExecutorError::Assembly(e.to_string()))
}

pub fn execute_with_proof(masm: &str, inputs: &[u64]) -> Result<ExecutionArtifacts, ExecutorError> {
    let program = assemble_program(masm)?;
    let stack_inputs = StackInputs::try_from_ints(inputs.iter().copied())
        .map_err(|e| ExecutorError::Input(e.to_string()))?;

    let mut host = DefaultHost::default();
    let (stack_outputs, proof) = prove(
        &program,
        stack_inputs.clone(),
        AdviceInputs::default(),
        &mut host,
        ProvingOptions::default(),
    )
    .map_err(|e| ExecutorError::Execution(e.to_string()))?;

    Ok(ExecutionArtifacts {
        stack_outputs: stack_outputs.as_int_vec(),
        proof_bytes: proof.to_bytes(),
        program_info: program.clone().into(),
        stack_inputs,
    })
}

pub fn execute_masm_file_with_proof(
    masm_path: &str,
    inputs: &[u64],
) -> Result<ExecutionArtifacts, ExecutorError> {
    let masm = fs::read_to_string(masm_path)
        .map_err(|e| ExecutorError::Input(format!("unable to read MASM file {masm_path}: {e}")))?;
    execute_with_proof(&masm, inputs)
}

pub fn verify_execution(
    program_info: ProgramInfo,
    stack_inputs: StackInputs,
    stack_outputs: StackOutputs,
    proof_bytes: &[u8],
) -> Result<u32, ExecutorError> {
    let proof = ExecutionProof::from_bytes(proof_bytes)
        .map_err(|e| ExecutorError::ProofDeserialization(e.to_string()))?;

    verify(program_info, stack_inputs, stack_outputs, proof).map_err(map_verification_error)
}

fn map_verification_error(err: VerificationError) -> ExecutorError {
    ExecutorError::Verification(err.to_string())
}

pub fn stack_outputs_from_ints(outputs: &[u64]) -> Result<StackOutputs, ExecutorError> {
    StackOutputs::try_from_ints(outputs.iter().copied())
        .map_err(|e| ExecutorError::Input(e.to_string()))
}

pub type ProgramId = u64;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum EventKind {
    Set,
    Delete,
    Get,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum EngineEvent {
    Set { key: u64, value: u64 },
    Delete { key: u64 },
    Get { key: u64, value: Option<u64> },
}

#[derive(Debug, Clone)]
pub struct TaskInput {
    pub inputs: Vec<u64>,
}

#[derive(Debug, Clone)]
pub struct TaskResult {
    pub runs: Vec<ExecutionArtifacts>,
    pub events: Vec<EngineEvent>,
    pub final_state: std::collections::HashMap<u64, u64>,
}

type EventHandler =
    Arc<dyn Fn(&EngineEvent, &mut std::collections::HashMap<u64, u64>) + Send + Sync + 'static>;
type EventDecoder = Arc<dyn Fn(&ExecutionArtifacts) -> Vec<EngineEvent> + Send + Sync + 'static>;

#[derive(Debug)]
pub enum EngineError {
    ProgramNotFound(ProgramId),
    QueueClosed,
    Executor(ExecutorError),
}

impl Display for EngineError {
    fn fmt(&self, f: &mut Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::ProgramNotFound(id) => write!(f, "program with id {id} was not found"),
            Self::QueueClosed => write!(f, "program queue is closed"),
            Self::Executor(err) => write!(f, "{err}"),
        }
    }
}
impl std::error::Error for EngineError {}
impl From<ExecutorError> for EngineError {
    fn from(value: ExecutorError) -> Self {
        Self::Executor(value)
    }
}

enum QueueMode {
    Single(TaskInput),
    Batch(Vec<TaskInput>),
}

struct QueuedTask {
    mode: QueueMode,
    responder: mpsc::Sender<Result<TaskResult, EngineError>>,
}

struct ProgramWorker {
    sender: mpsc::Sender<QueuedTask>,
}

pub struct ExecutionEngine {
    next_program_id: AtomicU64,
    workers: Arc<Mutex<std::collections::HashMap<ProgramId, ProgramWorker>>>,
    state: Arc<Mutex<std::collections::HashMap<u64, u64>>>,
    handlers: Arc<Mutex<std::collections::HashMap<EventKind, Vec<EventHandler>>>>,
    decoder: Arc<Mutex<EventDecoder>>,
}

impl Default for ExecutionEngine {
    fn default() -> Self {
        Self::new()
    }
}

impl ExecutionEngine {
    pub fn new() -> Self {
        Self {
            next_program_id: AtomicU64::new(1),
            workers: Arc::new(Mutex::new(std::collections::HashMap::new())),
            state: Arc::new(Mutex::new(std::collections::HashMap::new())),
            handlers: Arc::new(Mutex::new(std::collections::HashMap::new())),
            decoder: Arc::new(Mutex::new(Arc::new(default_event_decoder))),
        }
    }

    pub fn register_event_handler<F>(&self, kind: EventKind, handler: F)
    where
        F: Fn(&EngineEvent, &mut std::collections::HashMap<u64, u64>) + Send + Sync + 'static,
    {
        let mut handlers = self.handlers.lock().expect("handlers lock poisoned");
        handlers.entry(kind).or_default().push(Arc::new(handler));
    }

    pub fn register_event_decoder<F>(&self, decoder: F)
    where
        F: Fn(&ExecutionArtifacts) -> Vec<EngineEvent> + Send + Sync + 'static,
    {
        let mut lock = self.decoder.lock().expect("decoder lock poisoned");
        *lock = Arc::new(decoder);
    }

    pub fn deploy_program(&self, masm: &str) -> Result<ProgramId, EngineError> {
        assemble_program(masm)?;

        let program_id = self.next_program_id.fetch_add(1, Ordering::SeqCst);
        let masm = masm.to_string();
        let (tx, rx) = mpsc::channel::<QueuedTask>();
        let shared_state = Arc::clone(&self.state);
        let handlers = Arc::clone(&self.handlers);
        let decoder = Arc::clone(&self.decoder);

        thread::spawn(move || {
            while let Ok(task) = rx.recv() {
                let result = run_queued_task(
                    &masm,
                    task.mode,
                    &shared_state,
                    &handlers,
                    decoder.lock().expect("decoder lock poisoned").clone(),
                );
                let _ = task.responder.send(result);
            }
        });

        let worker = ProgramWorker { sender: tx };
        self.workers
            .lock()
            .expect("workers lock poisoned")
            .insert(program_id, worker);
        Ok(program_id)
    }

    pub fn deploy_program_from_path(&self, masm_path: &str) -> Result<ProgramId, EngineError> {
        let masm = fs::read_to_string(masm_path)
            .map_err(|e| EngineError::Executor(ExecutorError::Input(e.to_string())))?;
        self.deploy_program(&masm)
    }

    pub fn submit_task(
        &self,
        program_id: ProgramId,
        input: TaskInput,
    ) -> Result<TaskResult, EngineError> {
        self.submit(program_id, QueueMode::Single(input))
    }

    pub fn submit_batch(
        &self,
        program_id: ProgramId,
        batch: Vec<TaskInput>,
    ) -> Result<TaskResult, EngineError> {
        self.submit(program_id, QueueMode::Batch(batch))
    }

    fn submit(&self, program_id: ProgramId, mode: QueueMode) -> Result<TaskResult, EngineError> {
        let workers = self.workers.lock().expect("workers lock poisoned");
        let worker = workers
            .get(&program_id)
            .ok_or(EngineError::ProgramNotFound(program_id))?;

        let (tx, rx) = mpsc::channel();
        worker
            .sender
            .send(QueuedTask {
                mode,
                responder: tx,
            })
            .map_err(|_| EngineError::QueueClosed)?;

        rx.recv().map_err(|_| EngineError::QueueClosed)?
    }
}

fn run_queued_task(
    masm: &str,
    mode: QueueMode,
    shared_state: &Arc<Mutex<std::collections::HashMap<u64, u64>>>,
    handlers: &Arc<Mutex<std::collections::HashMap<EventKind, Vec<EventHandler>>>>,
    decoder: EventDecoder,
) -> Result<TaskResult, EngineError> {
    let mut runs = Vec::new();
    let mut events = Vec::new();
    let items = match mode {
        QueueMode::Single(t) => vec![t],
        QueueMode::Batch(v) => v,
    };

    for task in items {
        let artifacts = execute_with_proof(masm, &task.inputs)?;
        let decoded = decoder(&artifacts);
        apply_events(shared_state, handlers, &decoded);
        events.extend(decoded);
        runs.push(artifacts);
    }

    let final_state = shared_state.lock().expect("state lock poisoned").clone();
    Ok(TaskResult {
        runs,
        events,
        final_state,
    })
}

fn apply_events(
    state: &Arc<Mutex<std::collections::HashMap<u64, u64>>>,
    handlers: &Arc<Mutex<std::collections::HashMap<EventKind, Vec<EventHandler>>>>,
    events: &[EngineEvent],
) {
    let handlers_guard = handlers.lock().expect("handlers lock poisoned");
    let mut state_guard = state.lock().expect("state lock poisoned");

    for event in events {
        match event {
            EngineEvent::Set { key, value } => {
                state_guard.insert(*key, *value);
                if let Some(hs) = handlers_guard.get(&EventKind::Set) {
                    for h in hs {
                        h(event, &mut state_guard);
                    }
                }
            }
            EngineEvent::Delete { key } => {
                state_guard.remove(key);
                if let Some(hs) = handlers_guard.get(&EventKind::Delete) {
                    for h in hs {
                        h(event, &mut state_guard);
                    }
                }
            }
            EngineEvent::Get { key, .. } => {
                if let Some(hs) = handlers_guard.get(&EventKind::Get) {
                    for h in hs {
                        h(event, &mut state_guard);
                    }
                }
                let _ = state_guard.get(key);
            }
        }
    }
}

fn default_event_decoder(artifacts: &ExecutionArtifacts) -> Vec<EngineEvent> {
    let values = &artifacts.stack_outputs;
    if values.len() < 4 {
        return vec![];
    }
    let count = values[0] as usize;
    let mut events = Vec::new();
    for i in 0..count {
        let base = 1 + i * 3;
        if base + 2 >= values.len() {
            break;
        }
        let op = values[base];
        let key = values[base + 1];
        let value = values[base + 2];
        let event = match op {
            1 => EngineEvent::Set { key, value },
            2 => EngineEvent::Delete { key },
            3 => EngineEvent::Get { key, value: None },
            _ => continue,
        };
        events.push(event);
    }
    events
}
