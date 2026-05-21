use crate::prelude::*;
use crate::globals::GLOBAL_DB;
use crate::bridge::messaging::log;

pub struct WasmLock {
    pub mut_: Mutex<()>,
}

impl WasmLock {
    pub fn generate() -> Self {
        WasmLock {
            mut_: Mutex::new(()),
        }
    }
}

pub struct WasmTask {
    pub id: i32,
    pub name: String,
    pub inputs: Arc<Mutex<HashMap<i32, (bool, Arc<Mutex<WasmTask>>)>>>,
    pub outputs: Arc<Mutex<HashMap<i32, Arc<Mutex<WasmTask>>>>>,
    pub vm_index: i32,
    pub started: bool,
}

impl WasmTask {
    pub fn new() -> Self {
        WasmTask {
            id: 0,
            name: String::new(),
            inputs: Arc::new(Mutex::new(HashMap::new())),
            outputs: Arc::new(Mutex::new(HashMap::new())),
            vm_index: 0,
            started: false,
        }
    }
}

pub struct ChainTrx {
    pub machine_id: String,
    pub key: String,
    pub input: String,
    pub user_id: String,
    pub callback_id: String,
}

impl ChainTrx {
    pub fn new(
        machine_id: String,
        key: String,
        input: String,
        user_id: String,
        callback_id: String,
    ) -> Self {
        ChainTrx {
            machine_id,
            key,
            input,
            user_id,
            callback_id,
        }
    }
}

#[derive(Clone)]
pub struct WasmDbOp {
    pub type_: String,
    pub key: String,
    pub val: String,
}

impl WasmDbOp {
    pub fn new() -> Self {
        WasmDbOp {
            type_: String::new(),
            key: String::new(),
            val: String::new(),
        }
    }
}

pub struct Trx {
    pub write_options: WriteOptions,
    pub read_options: ReadOptions,
    pub txn_options: TransactionOptions,
    pub store: BTreeMap<String, String>,
    pub newly_created: BTreeMap<String, bool>,
    pub newly_deleted: BTreeMap<String, bool>,
    pub ops: Vec<WasmDbOp>,
}

impl Trx {
    pub fn new() -> Self {
        Trx {
            write_options: WriteOptions::default(),
            read_options: ReadOptions::default(),
            txn_options: TransactionOptions::default(),
            store: BTreeMap::new(),
            newly_created: BTreeMap::new(),
            newly_deleted: BTreeMap::new(),
            ops: Vec::new(),
        }
    }

    pub fn get_bytes_of_str(&self, str_val: String) -> Vec<u8> {
        let mut bytes: Vec<u8> = str_val.into_bytes();
        bytes.push(0);
        bytes
    }

    pub fn put(&mut self, key: String, val: String) {
        self.ops.push(WasmDbOp {
            type_: "put".to_string(),
            key: key.clone(),
            val: val.clone(),
        });
        self.store.insert(key.clone(), val);
        self.newly_created.insert(key.clone(), true);
        self.newly_deleted.remove(&key);
    }

    pub fn get_by_prefix(&mut self, prefix: String) -> Vec<String> {
        let mut result = Vec::new();
        for (key, value) in &self.store {
            if key.starts_with(&prefix) {
                result.push(value.clone());
            }
        }
        let db = GLOBAL_DB.lock().unwrap();
        for t in db.prefix_iterator(prefix.as_bytes().to_vec().as_slice()) {
            let item = t.unwrap();
            let key = str::from_utf8(item.0.to_vec().as_slice())
                .unwrap()
                .to_string();
            let val = str::from_utf8(item.1.to_vec().as_slice())
                .unwrap()
                .to_string();
            if !self.store.contains_key(&key) && !self.store.contains_key(&key) {
                result.push(val);
            }
        }
        result
    }

    pub fn get(&mut self, key: String) -> String {
        if let Some(value) = self.store.get(&key) {
            value.clone()
        } else if self.newly_deleted.contains_key(&key) {
            return "".to_string();
        } else {
            let db = GLOBAL_DB.lock().unwrap();
            let raw_val = db.get(key.as_bytes().to_vec().as_slice());
            let value: String;
            if raw_val.is_ok() {
                value = if let Some(val) = raw_val.unwrap() {
                    str::from_utf8(val.as_slice()).unwrap().to_string()
                } else {
                    "".to_string()
                };
            } else {
                value = "".to_string();
            }
            self.store.insert(key.clone(), value.clone());
            value
        }
    }

    pub fn del(&mut self, key: String) {
        let k = String::from(key);
        self.ops.push(WasmDbOp {
            type_: "del".to_string(),
            key: k.clone(),
            val: String::new(),
        });
        self.store.remove(&k);
        self.newly_created.remove(&k);
        self.newly_deleted.insert(k, true);
    }

    pub fn commit_as_offchain(&mut self) {
        let global_db = GLOBAL_DB.lock().unwrap();
        let trx = global_db.transaction();
        for op in &self.ops {
            if op.type_ == "put" {
                trx.put(&op.key, &op.val).unwrap();
            } else if op.type_ == "del" {
                trx.delete(&op.key).unwrap();
            }
        }
        trx.commit().unwrap();
        log("committed transaction successfully.".to_string());
    }

    pub fn dummy_commit(&mut self) {}
}

pub struct WasmThreadPool {
    tasks_: Arc<Mutex<VecDeque<Box<dyn FnOnce() + Send>>>>,
    cv_: Arc<Condvar>,
    stop_: Arc<AtomicBool>,
}

impl WasmThreadPool {
    pub fn generate(num_threads: Option<usize>) -> Self {
        let num_threads = num_threads.unwrap_or_else(|| {
            thread::available_parallelism()
                .map(|n| n.get())
                .unwrap_or(1)
        });
        let vd: VecDeque<Box<dyn FnOnce() + Send>> = VecDeque::new();
        let tasks_: Arc<Mutex<VecDeque<Box<dyn FnOnce() + Send>>>> = Arc::new(Mutex::new(vd));
        let cv_ = Arc::new(Condvar::new());
        let stop_ = Arc::new(AtomicBool::new(false));

        for _i in 0..num_threads {
            let tasks_clone = Arc::clone(&tasks_);
            let cv_clone = Arc::clone(&cv_);
            let stop_clone = Arc::clone(&stop_);

            thread::spawn(move || loop {
                let task = {
                    let tasks = tasks_clone.lock().unwrap();
                    let mut _mg: std::sync::MutexGuard<'_, VecDeque<Box<dyn FnOnce() + Send>>> =
                        cv_clone
                            .wait_while(tasks, |tasks| {
                                tasks.is_empty() && !stop_clone.load(Ordering::Relaxed)
                            })
                            .unwrap();
                    if stop_clone.load(Ordering::Relaxed) && _mg.is_empty() {
                        return;
                    }

                    _mg.pop_front()
                };

                if let Some(task) = task {
                    task();
                }
            });
        }

        WasmThreadPool { tasks_, cv_, stop_ }
    }

    pub fn stop_pool(&mut self) {
        self.stop_.store(true, Ordering::Relaxed);
        self.cv_.notify_all();
    }

    pub fn enqueue<F>(&self, task: F)
    where
        F: FnOnce() + Send + 'static,
    {
        {
            let mut tasks = self.tasks_.lock().unwrap();
            tasks.push_back(Box::new(task));
            drop(tasks);
        }
        self.cv_.notify_one();
    }
}
