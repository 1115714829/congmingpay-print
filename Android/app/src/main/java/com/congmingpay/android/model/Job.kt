package com.congmingpay.android.model

/**
 * 打印任务状态。
 */
object JobStatus {
    const val QUEUED = "queued"
    const val PRINTING = "printing"
    const val WAITING = "waiting"   // 网络异常，等待重试
    const val DONE = "done"
    const val FAILED = "failed"

    fun label(status: String): String = when (status) {
        QUEUED -> "排队中"
        PRINTING -> "打印中"
        WAITING -> "等待重试"
        DONE -> "已完成"
        FAILED -> "失败"
        else -> status
    }

    /** 任务是否仍在进行（排队 / 打印中 / 等待重试） */
    fun active(status: String): Boolean =
        status == QUEUED || status == PRINTING || status == WAITING
}

/**
 * 打印任务（展示用）。多份打印（pCopy）= 多个任务，每个 Job 恒为一份。
 */
data class Job(
    val no: Int,
    val doc: String,
    val printer: String,
    val status: String,
    val time: String,
    val err: String = ""
)
